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
