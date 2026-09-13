package bot

import (
	"context"
	"github.com/automuteus/automuteus/v8/pkg/game"
	"github.com/automuteus/automuteus/v8/pkg/premium"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/automuteus/automuteus/v8/pkg/task"
	"github.com/bsm/redislock"
	"github.com/bwmarrin/discordgo"
	"log"
	"strconv"
	"time"
)

type HandlePriority int

const (
	NoPriority    HandlePriority = 0
	AlivePriority HandlePriority = 1
	DeadPriority  HandlePriority = 2
)

func (bot *Bot) applyToSingle(dgs *GameState, userID string, mute, deaf bool) error {
	prem, days, _ := bot.PostgresInterface.GetGuildOrUserPremiumStatus(bot.official, nil, dgs.GuildID, "")
	premTier := premium.FreeTier
	if !premium.IsExpired(prem, days) {
		premTier = prem
	}
	uid, _ := strconv.ParseUint(userID, 10, 64)
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
	// nil lock because this is an override; we don't care about legitimately obtaining the lock
	return bot.TokenProvider.ModifyUsers(dgs.GuildID, dgs.ConnectCode, req, nil)
}

func (bot *Bot) applyToAll(dgs *GameState, mute, deaf bool) error {
	g, err := bot.PrimarySession.State.Guild(dgs.GuildID)
	if err != nil {
		return err
	}

	var users []task.UserModify

	for _, voiceState := range g.VoiceStates {
		userData, err := dgs.GetUser(voiceState.UserID)
		if err != nil {
			// the User doesn't exist in our userdata cache; add them
			added := false
			userData, added = dgs.checkCacheAndAddUser(g, bot.PrimarySession, voiceState.UserID)
			if !added {
				continue
			}
		}

		tracked := voiceState.ChannelID != "" && dgs.VoiceChannel == voiceState.ChannelID

		_, linked := dgs.GameData.GetByName(userData.InGameName)
		// only actually tracked if we're in a tracked channel AND linked to a player
		tracked = tracked && linked

		if tracked {
			uid, _ := strconv.ParseUint(userData.User.UserID, 10, 64)
			users = append(users, task.UserModify{
				UserID: uid,
				Mute:   mute,
				Deaf:   deaf,
			})
			log.Println("Forcibly applying mute/deaf to " + userData.User.UserID)
		}
	}
	if len(users) > 0 {
		prem, days, _ := bot.PostgresInterface.GetGuildOrUserPremiumStatus(bot.official, nil, dgs.GuildID, "")
		premTier := premium.FreeTier
		if !premium.IsExpired(prem, days) {
			premTier = prem
		}
		req := task.UserModifyRequest{
			Premium: premTier,
			Users:   users,
		}
		// nil lock because this is an override; we don't care about legitimately obtaining the lock
		return bot.TokenProvider.ModifyUsers(dgs.GuildID, dgs.ConnectCode, req, nil)
	}
	return nil
}

// handleTrackedMembers moves/mutes players according to the current game state
func (bot *Bot) handleTrackedMembers(sess *discordgo.Session, sett *settings.GuildSettings, delay int, handlePriority HandlePriority, gsr GameStateRequest) {

	lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(gsr, 10)
	if lock == nil || dgs == nil {
		log.Printf("Unable to update voice states for guild %s because the Redis game-state lock could not be obtained", gsr.GuildID)
		return
	}

	g, err := sess.State.Guild(dgs.GuildID)

	if err != nil || g == nil {
		lock.Release(ctx)
		return
	}

	var users []task.UserModify
	var hostTalkMembers []hostTalkVoiceMember

	priorityRequests := 0
	for _, voiceState := range g.VoiceStates {
		inTrackedVoiceChannel := voiceState.ChannelID != "" && dgs.VoiceChannel == voiceState.ChannelID

		userData, err := dgs.GetUser(voiceState.UserID)
		if err != nil {
			// the User doesn't exist in our userdata cache; add them
			added := false
			userData, added = dgs.checkCacheAndAddUser(g, sess, voiceState.UserID)
			if !added {
				hostTalkMembers = append(hostTalkMembers, hostTalkVoiceMember{
					UserID:                voiceState.UserID,
					InTrackedVoiceChannel: inTrackedVoiceChannel,
					WasHostTalkManaged:    dgs.GameStateMsg.HostTalkManagedUsers[voiceState.UserID],
				})
				continue
			}
		}

		tracked := inTrackedVoiceChannel

		auData, found := dgs.GameData.GetByName(userData.InGameName)
		// only actually tracked if we're in a tracked channel AND linked to a player
		var isAlive bool

		// only actually tracked if we're in a tracked channel AND linked to a player
		if !sett.GetMuteSpectator() {
			tracked = tracked && found
			isAlive = auData.IsAlive
		} else {
			if !found {
				// we just assume the spectator is dead
				isAlive = false
			} else {
				isAlive = auData.IsAlive
			}
		}
		shouldMute, shouldDeaf := sett.GetVoiceState(isAlive, tracked, dgs.GameData.GetPhase())

		hostTalkMember := hostTalkVoiceMember{
			UserID:                voiceState.UserID,
			InTrackedVoiceChannel: inTrackedVoiceChannel,
			NormalApplicable:      found || sett.GetMuteSpectator(),
			NormalMute:            shouldMute,
			NormalDeaf:            shouldDeaf,
			WasHostTalkManaged:    dgs.GameStateMsg.HostTalkManagedUsers[voiceState.UserID],
		}
		if handlePriority != NoPriority && ((handlePriority == AlivePriority && isAlive) || (handlePriority == DeadPriority && !isAlive)) {
			hostTalkMembers = append([]hostTalkVoiceMember{hostTalkMember}, hostTalkMembers...)
		} else {
			hostTalkMembers = append(hostTalkMembers, hostTalkMember)
		}

		incorrectMuteDeafenState := shouldMute != userData.ShouldBeMute || shouldDeaf != userData.ShouldBeDeaf

		// only issue a change if the User isn't in the right state already
		// nicksmatch can only be false if the in-game data is != nil, so the reference to .audata below is safe
		// check the userdata is linked here to not accidentally undeafen music bots, for example
		if incorrectMuteDeafenState && (found || sett.GetMuteSpectator()) {
			uid, _ := strconv.ParseUint(userData.User.UserID, 10, 64)
			userModify := task.UserModify{
				UserID: uid,
				Mute:   shouldMute,
				Deaf:   shouldDeaf,
			}

			if handlePriority != NoPriority && ((handlePriority == AlivePriority && isAlive) || (handlePriority == DeadPriority && !isAlive)) {
				users = append([]task.UserModify{userModify}, users...)
				priorityRequests++ // counter of how many elements on the front of the arr should be sent first
			} else {
				users = append(users, userModify)
			}
		}
	}

	expectedPhase := dgs.GameData.GetPhase()
	expectedHostTalkMode := dgs.GameStateMsg.HostTalkMode
	expectedHostTalkRevision := dgs.GameStateMsg.HostTalkRevision

	// We relinquish the game-state lock while waiting and while calling Discord.
	bot.RedisInterface.SetDiscordGameState(dgs, lock)

	if expectedHostTalkMode && dgs.GameStateMsg.LeaderID != "" {
		bot.handleHostTalkTrackedMembers(
			sess,
			delay,
			dgs.GuildID,
			dgs.ConnectCode,
			dgs.VoiceChannel,
			dgs.GameStateMsg.LeaderID,
			expectedPhase,
			expectedHostTalkMode,
			expectedHostTalkRevision,
			hostTalkMembers,
		)
		return
	}

	if len(users) == 0 {
		return
	}

	voiceLock := bot.RedisInterface.LockVoiceChanges(dgs.ConnectCode, time.Second*time.Duration(delay+1))
	if voiceLock == nil {
		log.Printf("Skipped overlapping voice update for game %s", dgs.ConnectCode)
		return
	}

	if delay > 0 {
		log.Printf("Sleeping for %d seconds before applying changes to users\n", delay)
		time.Sleep(time.Second * time.Duration(delay))
	}

	// A newer phase may have arrived while this request was delayed. Never apply an
	// obsolete mute plan after the game has already moved on.
	latest := bot.RedisInterface.GetReadOnlyDiscordGameState(GameStateRequest{
		GuildID:     dgs.GuildID,
		ConnectCode: dgs.ConnectCode,
	})
	if latest == nil ||
		!latest.Running ||
		latest.GameData.GetPhase() != expectedPhase ||
		!hostTalkVoiceStateMatches(
			latest,
			dgs.GuildID,
			dgs.ConnectCode,
			expectedHostTalkMode,
			expectedHostTalkRevision,
		) {
		if voiceLock != nil {
			_ = voiceLock.Release(context.Background())
		}
		log.Printf("Skipped stale voice update for game %s", dgs.ConnectCode)
		return
	}

	if len(users) > 0 {
		prem, days, _ := bot.PostgresInterface.GetGuildOrUserPremiumStatus(bot.official, nil, dgs.GuildID, "")
		premTier := premium.FreeTier
		if !premium.IsExpired(prem, days) {
			premTier = prem
		}

		if priorityRequests > 0 {
			req := task.UserModifyRequest{
				Premium: premTier,
				Users:   users[:priorityRequests],
			}
			// no lock; we're not done yet
			err := bot.issueMutesAndRecord(dgs.GuildID, dgs.ConnectCode, req, nil)
			if err != nil {
				log.Println(err)
			} else {
				log.Println("Successfully finished issuing high priority mutes")
			}
			rem := users[priorityRequests:]
			if len(rem) > 0 {
				req = task.UserModifyRequest{
					Premium: premTier,
					Users:   rem,
				}
				err := bot.issueMutesAndRecord(dgs.GuildID, dgs.ConnectCode, req, voiceLock)
				if err != nil {
					log.Println(err)
				}
			} else if voiceLock != nil {
				voiceLock.Release(context.Background())
			}
		} else {
			// no priority; issue all at once
			log.Println("Issuing mutes/deafens with no particular priority")
			req := task.UserModifyRequest{
				Premium: premTier,
				Users:   users,
			}
			err := bot.issueMutesAndRecord(dgs.GuildID, dgs.ConnectCode, req, voiceLock)
			if err != nil {
				log.Println(err)
			}
		}
	}
}

// handleHostTalkTrackedMembers applies HostTalk ON as a separate runtime path.
// Normal AutoMute remains unchanged when HostTalk is OFF or no leader is available.
func (bot *Bot) handleHostTalkTrackedMembers(
	sess *discordgo.Session,
	delay int,
	guildID string,
	connectCode string,
	voiceChannelID string,
	leaderID string,
	expectedPhase game.Phase,
	expectedMode bool,
	expectedRevision uint64,
	members []hostTalkVoiceMember,
) {
	if bot == nil ||
		bot.RedisInterface == nil ||
		bot.TokenProvider == nil ||
		sess == nil ||
		sess.State == nil ||
		!expectedMode ||
		leaderID == "" {
		return
	}

	resolvedMembers := map[string]*discordgo.Member{}
	resolveMember := func(userID string) (*discordgo.Member, error) {
		if member, ok := resolvedMembers[userID]; ok {
			return member, nil
		}
		member, err := sess.GuildMember(guildID, userID)
		if err != nil {
			return nil, err
		}
		if member != nil {
			resolvedMembers[userID] = member
		}
		return member, nil
	}

	observe := func() (map[string]hostTalkVoiceObservation, bool) {
		guild, err := sess.State.Guild(guildID)
		if err != nil || guild == nil {
			return nil, false
		}
		return observeHostTalkGuildVoiceMembersWithResolver(
			guild,
			voiceChannelID,
			resolveMember,
		), true
	}

	stateStillCurrent := func() bool {
		latest := bot.RedisInterface.GetReadOnlyDiscordGameState(GameStateRequest{
			GuildID:     guildID,
			ConnectCode: connectCode,
		})
		return latest != nil &&
			latest.Running &&
			latest.GameData.GetPhase() == expectedPhase &&
			hostTalkVoiceStateMatches(
				latest,
				guildID,
				connectCode,
				expectedMode,
				expectedRevision,
			)
	}

	initialObservations, ok := observe()
	if !ok {
		return
	}
	initialMembers := refreshHostTalkVoiceMembers(members, initialObservations)
	initialPlans := planHostTalkVoiceBatch(expectedMode, leaderID, initialMembers)
	if len(initialPlans) == 0 {
		return
	}

	voiceLock := bot.RedisInterface.LockVoiceChanges(
		connectCode,
		time.Second*time.Duration(delay+1),
	)
	if voiceLock == nil {
		log.Printf("Skipped overlapping HostTalk voice update for game %s", connectCode)
		return
	}

	voiceLockOwned := true
	defer func() {
		if voiceLockOwned {
			_ = voiceLock.Release(context.Background())
		}
	}()

	if delay > 0 {
		log.Printf("Sleeping for %d seconds before applying HostTalk changes to users\n", delay)
		time.Sleep(time.Second * time.Duration(delay))
	}

	if !stateStillCurrent() {
		log.Printf("Skipped stale HostTalk voice update for game %s", connectCode)
		return
	}

	preSendObservations, ok := observe()
	if !ok {
		return
	}
	currentMembers := refreshHostTalkVoiceMembers(members, preSendObservations)
	plans := planHostTalkVoiceBatch(expectedMode, leaderID, currentMembers)
	alreadyApplied, pending := partitionHostTalkVoicePlans(plans, preSendObservations)
	alreadyApplied = filterHostTalkVoiceRecords(alreadyApplied, preSendObservations, true)
	pending = filterHostTalkPendingVoicePlans(pending, preSendObservations)

	if len(alreadyApplied) == 0 && len(pending) == 0 {
		return
	}

	requestUsers := make([]task.UserModify, 0, len(pending))
	sentPending := make([]hostTalkPendingVoicePlan, 0, len(pending))
	for _, candidate := range pending {
		uid, err := strconv.ParseUint(candidate.Plan.UserID, 10, 64)
		if err != nil {
			log.Printf("Skipped HostTalk voice request for invalid Discord user ID %q: %v", candidate.Plan.UserID, err)
			continue
		}
		requestUsers = append(requestUsers, task.UserModify{
			UserID: uid,
			Mute:   candidate.Plan.Mute,
			Deaf:   candidate.Plan.Deaf,
		})
		sentPending = append(sentPending, candidate)
	}

	var sendErr error
	if len(requestUsers) > 0 {
		// Re-check game identity/phase/mode/revision immediately before Discord.
		if !stateStillCurrent() {
			log.Printf("Skipped HostTalk send after a newer state arrived for game %s", connectCode)
			return
		}

		prem, days, _ := bot.PostgresInterface.GetGuildOrUserPremiumStatus(
			bot.official,
			nil,
			guildID,
			"",
		)
		premTier := premium.FreeTier
		if !premium.IsExpired(prem, days) {
			premTier = prem
		}

		req := task.UserModifyRequest{
			Premium: premTier,
			Users:   requestUsers,
		}

		// ModifyUsers owns/relinquishes voiceLock once called.
		voiceLockOwned = false
		sendErr = bot.TokenProvider.ModifyUsers(
			guildID,
			connectCode,
			req,
			voiceLock,
		)
		if sendErr != nil {
			log.Printf("HostTalk Discord voice update failed for game %s: %v", connectCode, sendErr)
		}
	}

	// Revalidate human/bot identity and VC side again before recording.
	postSendObservations, ok := observe()
	if !ok {
		return
	}

	records := filterHostTalkVoiceRecords(
		alreadyApplied,
		postSendObservations,
		true,
	)

	if sendErr == nil && len(requestUsers) > 0 {
		records = append(
			records,
			recordsFromSuccessfulHostTalkPending(
				sentPending,
				postSendObservations,
			)...,
		)
	}

	if len(records) == 0 {
		return
	}

	if err := bot.recordHostTalkSuccessfulVoiceRecords(
		GameStateRequest{
			GuildID:     guildID,
			ConnectCode: connectCode,
		},
		expectedPhase,
		expectedMode,
		expectedRevision,
		records,
	); err != nil {
		log.Printf("HostTalk voice state applied but bookkeeping was not committed for game %s: %v", connectCode, err)
	}
}

func (bot *Bot) issueMutesAndRecord(guildID, connectCode string, req task.UserModifyRequest, lock *redislock.Lock) error {
	if err := bot.TokenProvider.ModifyUsers(guildID, connectCode, req, lock); err != nil {
		return err
	}

	// Only record the desired state after Discord accepted the modification.
	// If the request fails, the old state remains and the next game event retries it.
	stateLock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(GameStateRequest{
		GuildID:     guildID,
		ConnectCode: connectCode,
	}, 5)
	if stateLock == nil || dgs == nil {
		log.Printf("Voice changes succeeded for game %s, but the state lock could not be obtained to record them", connectCode)
		return nil
	}

	for _, modification := range req.Users {
		userID := strconv.FormatUint(modification.UserID, 10)
		userData, err := dgs.GetUser(userID)
		if err != nil {
			continue
		}
		userData.SetShouldBeMuteDeaf(modification.Mute, modification.Deaf)
		dgs.UpdateUserData(userID, userData)
	}
	bot.RedisInterface.SetDiscordGameState(dgs, stateLock)
	return nil
}
