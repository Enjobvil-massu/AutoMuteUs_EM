package bot

import (
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"log"
	"strconv"
	"time"

	"github.com/automuteus/automuteus/v8/pkg/premium"
	"github.com/automuteus/automuteus/v8/pkg/task"

	"github.com/bwmarrin/discordgo"
)

// voiceStateChange handles more edge-case behavior for users moving between voice channels, and catches when
// relevant discord api requests are fully applied successfully. Otherwise, we can issue multiple requests for
// the same mute/unmute, erroneously
func voiceStateGameChannels(m *discordgo.VoiceStateUpdate) []string {
	if m == nil || m.VoiceState == nil {
		return nil
	}

	channels := make([]string, 0, 2)
	appendUnique := func(channelID string) {
		if channelID == "" {
			return
		}
		for _, existing := range channels {
			if existing == channelID {
				return
			}
		}
		channels = append(channels, channelID)
	}

	// Process the channel the user left first. This ensures a linked player is
	// unmuted when they leave or move away from the tracked voice channel.
	if m.BeforeUpdate != nil {
		appendUnique(m.BeforeUpdate.ChannelID)
	}
	appendUnique(m.ChannelID)

	return channels
}

// handleVoiceStateChange handles users joining, leaving, or moving between
// voice channels. Both the previous and current channels are considered so a
// user who leaves the tracked channel is not left server-muted.
func voiceStateNeedsDiscordUpdate(found, desiredMute, desiredDeaf, actualMute, actualDeaf bool) bool {
	return found && (desiredMute != actualMute || desiredDeaf != actualDeaf)
}

func (bot *Bot) handleVoiceStateChange(s *discordgo.Session, m *discordgo.VoiceStateUpdate) {
	if s == nil || m == nil || m.VoiceState == nil {
		return
	}

	snowFlakeLock := bot.RedisInterface.LockSnowflake(m.GuildID + ":" + m.UserID + ":" + m.SessionID)
	if snowFlakeLock == nil {
		return
	}
	defer snowFlakeLock.Release(ctx)

	for _, voiceChannelID := range voiceStateGameChannels(m) {
		bot.handleVoiceStateChangeForGame(s, m, voiceChannelID)
	}
}

func (bot *Bot) handleVoiceStateChangeForGame(s *discordgo.Session, m *discordgo.VoiceStateUpdate, voiceChannelID string) {
	if bot.handleHostTalkVoiceStateChangeForGame(s, m, voiceChannelID) {
		return
	}

	gsr := GameStateRequest{
		GuildID:      m.GuildID,
		VoiceChannel: voiceChannelID,
	}

	// Do not create an empty game state for unrelated voice channels.
	if bot.RedisInterface.getDiscordGameStateKey(gsr) == "" {
		return
	}

	stateLock, dgs := bot.RedisInterface.GetDiscordGameStateAndLock(gsr)
	if stateLock == nil {
		return
	}
	if dgs == nil {
		_ = stateLock.Release(ctx)
		return
	}

	g, err := s.State.Guild(dgs.GuildID)
	if err != nil || g == nil {
		bot.RedisInterface.SetDiscordGameState(nil, stateLock)
		return
	}

	userData, err := dgs.GetUser(m.UserID)
	if err != nil {
		var added bool
		userData, added = dgs.checkCacheAndAddUser(g, s, m.UserID)
		if !added {
			bot.RedisInterface.SetDiscordGameState(nil, stateLock)
			return
		}
	}

	tracked := m.ChannelID != "" && dgs.VoiceChannel == m.ChannelID
	auData, found := dgs.GameData.GetByName(userData.InGameName)

	sett := bot.StorageInterface.GetGuildSettings(m.GuildID)
	var isAlive bool
	if !sett.GetMuteSpectator() {
		tracked = tracked && found
		isAlive = auData.IsAlive
	} else if found {
		isAlive = auData.IsAlive
	}

	mute, deaf := sett.GetVoiceState(isAlive, tracked, dgs.GameData.GetPhase())
	cacheNeedsUpdate := found && (userData.ShouldBeDeaf != deaf || userData.ShouldBeMute != mute)
	needsDiscordUpdate := voiceStateNeedsDiscordUpdate(found, mute, deaf, m.Mute, m.Deaf)

	if !needsDiscordUpdate {
		// Discord is already in the desired state. Keep the Redis cache in sync so
		// a later voice event is not skipped because of stale ShouldBe values.
		if cacheNeedsUpdate {
			userData.SetShouldBeMuteDeaf(mute, deaf)
			dgs.UpdateUserData(m.UserID, userData)
		}
		bot.RedisInterface.SetDiscordGameState(dgs, stateLock)
		return
	}

	if !dgs.Running || dgs.ConnectCode == "" {
		bot.RedisInterface.SetDiscordGameState(dgs, stateLock)
		return
	}

	uid, err := strconv.ParseUint(m.UserID, 10, 64)
	if err != nil {
		log.Printf("Unable to parse Discord user ID %q while handling a voice-state change: %v", m.UserID, err)
		bot.RedisInterface.SetDiscordGameState(dgs, stateLock)
		return
	}

	guildID := dgs.GuildID
	connectCode := dgs.ConnectCode
	bot.RedisInterface.SetDiscordGameState(dgs, stateLock)

	prem, days, _ := bot.PostgresInterface.GetGuildOrUserPremiumStatus(bot.official, nil, guildID, "")
	premTier := premium.FreeTier
	if !premium.IsExpired(prem, days) {
		premTier = prem
	}

	voiceLock := bot.RedisInterface.LockVoiceChanges(connectCode, time.Second)
	if voiceLock == nil {
		log.Printf("Skipped overlapping voice-state update for game %s", connectCode)
		return
	}

	req := task.UserModifyRequest{
		Premium: premTier,
		Users: []task.UserModify{
			{
				UserID: uid,
				Mute:   mute,
				Deaf:   deaf,
			},
		},
	}
	if err := bot.issueMutesAndRecord(guildID, connectCode, req, voiceLock); err != nil {
		log.Println("error received while handling a voice-state change: ", err)
	}
}

// handleHostTalkVoiceStateChangeForGame returns true only when HostTalk
// owns this event. false deliberately falls through to the untouched
// existing normal AutoMute event path.
func (bot *Bot) handleHostTalkVoiceStateChangeForGame(
	s *discordgo.Session,
	m *discordgo.VoiceStateUpdate,
	voiceChannelID string,
) bool {
	if bot == nil ||
		bot.RedisInterface == nil ||
		s == nil ||
		m == nil ||
		m.VoiceState == nil {
		return false
	}

	gsr := GameStateRequest{
		GuildID:      m.GuildID,
		VoiceChannel: voiceChannelID,
	}

	if bot.RedisInterface.getDiscordGameStateKey(gsr) == "" {
		return false
	}

	stateLock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(gsr, 10)
	if stateLock == nil || dgs == nil {
		readOnly := bot.RedisInterface.GetReadOnlyDiscordGameState(gsr)
		if readOnly == nil {
			return false
		}

		probe := planHostTalkVoiceEvent(hostTalkVoiceEventInput{
			HostTalkMode:          readOnly.GameStateMsg.HostTalkMode,
			LeaderID:              readOnly.GameStateMsg.LeaderID,
			UserID:                m.UserID,
			InTrackedVoiceChannel: m.ChannelID != "" && readOnly.VoiceChannel == m.ChannelID,
			WasHostTalkManaged:    readOnly.GameStateMsg.HostTalkManagedUsers[m.UserID],
		})
		return probe.Handled
	}

	inTrackedVoiceChannel :=
		m.ChannelID != "" &&
			dgs.VoiceChannel == m.ChannelID

	wasHostTalkManaged :=
		dgs.GameStateMsg.HostTalkManagedUsers[m.UserID]

	ownership := planHostTalkVoiceEvent(hostTalkVoiceEventInput{
		HostTalkMode:          dgs.GameStateMsg.HostTalkMode,
		LeaderID:              dgs.GameStateMsg.LeaderID,
		UserID:                m.UserID,
		InTrackedVoiceChannel: inTrackedVoiceChannel,
		WasHostTalkManaged:    wasHostTalkManaged,
	})

	if !ownership.Handled {
		bot.RedisInterface.SetDiscordGameState(dgs, stateLock)
		return false
	}

	normalFound := false
	normalAlive := false
	if userData, err := dgs.GetUser(m.UserID); err == nil {
		if auData, found := dgs.GameData.GetByName(userData.InGameName); found {
			normalFound = true
			normalAlive = auData.IsAlive
		}
	}

	sett := bot.StorageInterface.GetGuildSettings(m.GuildID)
	expectedGuildID := dgs.GuildID
	expectedConnectCode := dgs.ConnectCode
	expectedVoiceChannelID := dgs.VoiceChannel
	expectedLeaderID := dgs.GameStateMsg.LeaderID
	expectedPhase := dgs.GameData.GetPhase()
	expectedMode := dgs.GameStateMsg.HostTalkMode
	expectedRevision := dgs.GameStateMsg.HostTalkRevision
	expectedRunning := dgs.Running

	bot.RedisInterface.SetDiscordGameState(dgs, stateLock)

	if !expectedRunning || expectedConnectCode == "" || s.State == nil {
		return true
	}

	resolveMember := func(userID string) (*discordgo.Member, error) {
		return s.GuildMember(expectedGuildID, userID)
	}

	observe := func() hostTalkVoiceEventObservation {
		guild, err := s.State.Guild(expectedGuildID)
		if err != nil || guild == nil {
			return hostTalkVoiceEventObservation{}
		}
		return observeHostTalkVoiceEventUser(
			guild,
			m.UserID,
			expectedVoiceChannelID,
			resolveMember,
		)
	}

	buildPlan := func(observation hostTalkVoiceEventObservation) hostTalkVoiceEventPlan {
		normalTracked := observation.InTrackedVoiceChannel
		if !sett.GetMuteSpectator() {
			normalTracked = normalTracked && normalFound
		}

		normalMute, normalDeaf :=
			sett.GetVoiceState(normalAlive, normalTracked, expectedPhase)

		return planHostTalkVoiceEvent(hostTalkVoiceEventInput{
			HostTalkMode:          expectedMode,
			LeaderID:              expectedLeaderID,
			UserID:                m.UserID,
			MemberKnown:           observation.MemberKnown,
			IsBot:                 observation.IsBot,
			InTrackedVoiceChannel: observation.InTrackedVoiceChannel,
			NormalApplicable:      normalFound || sett.GetMuteSpectator(),
			NormalMute:            normalMute,
			NormalDeaf:            normalDeaf,
			WasHostTalkManaged:    wasHostTalkManaged,
		})
	}

	stateStillCurrent := func() bool {
		latest := bot.RedisInterface.GetReadOnlyDiscordGameState(GameStateRequest{
			GuildID:     expectedGuildID,
			ConnectCode: expectedConnectCode,
		})
		return latest != nil &&
			latest.Running &&
			latest.GameData.GetPhase() == expectedPhase &&
			hostTalkVoiceStateMatches(
				latest,
				expectedGuildID,
				expectedConnectCode,
				expectedMode,
				expectedRevision,
			)
	}

	recordPlan := func(plan hostTalkVoiceEventPlan) {
		err := bot.recordHostTalkSuccessfulVoiceRecords(
			GameStateRequest{
				GuildID:     expectedGuildID,
				ConnectCode: expectedConnectCode,
			},
			expectedPhase,
			expectedMode,
			expectedRevision,
			[]hostTalkVoiceRecord{
				hostTalkVoiceRecordForEventPlan(plan, m.UserID),
			},
		)
		if err != nil {
			log.Printf(
				"Unable to record HostTalk voice-event state for game %s user %s: %v",
				expectedConnectCode,
				m.UserID,
				err,
			)
		}
	}

	observation := observe()
	plan := buildPlan(observation)

	// Once the original event belongs to HostTalk, never fall through to
	// normal AutoMute because a later snapshot changed while this handler
	// was running. The next Discord voice event will reconcile that state.
	if !plan.Handled || !plan.Apply {
		return true
	}

	if !stateStillCurrent() ||
		!hostTalkVoiceEventObservationMatchesPlan(plan, observation) {
		return true
	}

	if !hostTalkVoiceEventNeedsDiscordUpdate(plan, observation) {
		recordPlan(plan)
		return true
	}

	uid, err := strconv.ParseUint(m.UserID, 10, 64)
	if err != nil {
		log.Printf(
			"Unable to parse Discord user ID %q while handling HostTalk voice event: %v",
			m.UserID,
			err,
		)
		return true
	}

	prem, days, _ := bot.PostgresInterface.GetGuildOrUserPremiumStatus(
		bot.official,
		nil,
		expectedGuildID,
		"",
	)
	premTier := premium.FreeTier
	if !premium.IsExpired(prem, days) {
		premTier = prem
	}

	voiceLock := bot.RedisInterface.LockVoiceChanges(
		expectedConnectCode,
		time.Second,
	)
	if voiceLock == nil {
		log.Printf(
			"Skipped overlapping HostTalk voice-state update for game %s",
			expectedConnectCode,
		)
		return true
	}

	if !stateStillCurrent() {
		_ = voiceLock.Release(ctx)
		return true
	}

	preSendObservation := observe()
	preSendPlan := buildPlan(preSendObservation)
	if !preSendPlan.Handled ||
		!preSendPlan.Apply ||
		!hostTalkVoiceEventObservationMatchesPlan(preSendPlan, preSendObservation) {
		_ = voiceLock.Release(ctx)
		return true
	}

	if !hostTalkVoiceEventNeedsDiscordUpdate(preSendPlan, preSendObservation) {
		_ = voiceLock.Release(ctx)
		recordPlan(preSendPlan)
		return true
	}

	req := task.UserModifyRequest{
		Premium: premTier,
		Users: []task.UserModify{
			{
				UserID: uid,
				Mute:   preSendPlan.Mute,
				Deaf:   preSendPlan.Deaf,
			},
		},
	}

	// TokenProvider.ModifyUsers owns and releases voiceLock once called.
	err = bot.TokenProvider.ModifyUsers(
		expectedGuildID,
		expectedConnectCode,
		req,
		voiceLock,
	)
	if err != nil {
		log.Printf(
			"HostTalk voice-state Discord update failed for game %s user %s: %v",
			expectedConnectCode,
			m.UserID,
			err,
		)
		return true
	}

	// Discord's mute/deaf echo may lag a successful API request. Revalidate
	// identity and VC side, but do not require immediate mute/deaf equality.
	postSendObservation := observe()
	if !hostTalkVoiceEventObservationMatchesPlan(preSendPlan, postSendObservation) {
		return true
	}

	recordPlan(preSendPlan)
	return true
}

func (bot *Bot) handleGameStartMessage(guildID, textChannelID, voiceChannelID, userID string, sett *settings.GuildSettings, g *discordgo.Guild, connCode string) {
	lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLock(GameStateRequest{
		GuildID:     guildID,
		TextChannel: textChannelID,
		ConnectCode: connCode,
	})
	if lock == nil {
		log.Println("Couldn't obtain lock for DGS on game start...")
		return
	}
	dgs.GameData.Reset()

	dgs.UnlinkAllUsers()
	dgs.VoiceChannel = ""
	dgs.DeleteGameStateMsg(bot.PrimarySession, true)

	dgs.Running = true

	if voiceChannelID != "" {
		dgs.VoiceChannel = voiceChannelID
		for _, v := range g.VoiceStates {
			if v.ChannelID == voiceChannelID {
				dgs.checkCacheAndAddUser(g, bot.PrimarySession, v.UserID)
			}
		}
	}

	_ = dgs.CreateMessage(bot.PrimarySession, bot.gameStateResponse(dgs, sett), textChannelID, userID)

	// release the lock
	bot.RedisInterface.SetDiscordGameState(dgs, lock)
}
