package bot

import (
	"context"
	"errors"
	"fmt"
	"github.com/automuteus/automuteus/v8/bot/command"
	"github.com/automuteus/automuteus/v8/bot/tokenprovider"
	"github.com/automuteus/automuteus/v8/internal/server"
	"github.com/automuteus/automuteus/v8/pkg/amongus"
	"github.com/automuteus/automuteus/v8/pkg/discord"
	"github.com/automuteus/automuteus/v8/pkg/game"
	"github.com/automuteus/automuteus/v8/pkg/premium"
	"github.com/automuteus/automuteus/v8/pkg/rediskey"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	storageutils "github.com/automuteus/automuteus/v8/pkg/storage"
	"github.com/automuteus/automuteus/v8/pkg/token"
	"github.com/automuteus/automuteus/v8/storage"
	"github.com/bwmarrin/discordgo"
	"github.com/top-gg/go-dbl"
	"log"
	"os"
	"strconv"
	"sync"
	"time"
)

type Bot struct {
	version  string
	commit   string
	official bool
	url      string

	// mapping of socket connections to the game connect codes
	ConnsToGames map[string]string

	StatusEmojis AlivenessEmojis

	EndGameChannels map[string]chan EndGameMessage

	ChannelsMapLock sync.RWMutex
	SubscriberWG    sync.WaitGroup

	PrimarySession *discordgo.Session

	TokenProvider *tokenprovider.TokenProvider

	TopGGClient *dbl.Client

	RedisInterface *RedisInterface

	StorageInterface *storage.StorageInterface

	PostgresInterface *storageutils.PsqlInterface

	logPath string

	captureTimeout int
}

// MakeAndStartBot does what it sounds like
// TODO collapse these fields into proper structs?
func MakeAndStartBot(version, commit, botToken, topGGToken, url, emojiGuildID string, numShards, shardID int, redisInterface *RedisInterface, storageInterface *storage.StorageInterface, psql *storageutils.PsqlInterface, logPath string) *Bot {
	dg, err := discordgo.New("Bot " + botToken)
	if err != nil {
		log.Println("error creating Discord session,", err)
		return nil
	}

	if numShards > 1 {
		log.Printf("Identifying to the Discord API with %d total shards, and shard ID=%d\n", numShards, shardID)
		dg.ShardCount = numShards
		dg.ShardID = shardID
	}

	bot := Bot{
		version:      version,
		commit:       commit,
		official:     os.Getenv("AUTOMUTEUS_OFFICIAL") != "",
		url:          url,
		ConnsToGames: make(map[string]string),
		StatusEmojis: emptyStatusEmojis(),

		EndGameChannels:   make(map[string]chan EndGameMessage),
		ChannelsMapLock:   sync.RWMutex{},
		PrimarySession:    dg,
		RedisInterface:    redisInterface,
		StorageInterface:  storageInterface,
		PostgresInterface: psql,
		logPath:           logPath,
		captureTimeout:    GameTimeoutSeconds,
	}
	dg.LogLevel = discordgo.LogInformational

	dg.AddHandler(bot.handleVoiceStateChange)
	dg.AddHandler(bot.newGuild(emojiGuildID))
	dg.AddHandler(bot.leaveGuild)
	dg.AddHandler(bot.rateLimitEventCallback)
	// Slash commands
	dg.AddHandler(bot.handleInteractionCreate)

	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Println("Bot is now online according to discord Ready handler")
	})

	dg.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsGuildVoiceStates | discordgo.IntentsGuilds)

	token.WaitForToken(bot.RedisInterface.client, botToken)
	token.LockForToken(bot.RedisInterface.client, botToken)
	// Open a websocket connection to Discord and begin listening.
	err = dg.Open()
	if err != nil {
		log.Println("Could not connect Bot to the Discord Servers with error:", err)
		return nil
	}

	log.Println("Finished identifying to the Discord API. Now ready for incoming events")

	// ✅ 表示文言（新しい環境変数があれば優先、なければ従来の LISTENING を流用）
	playingText := os.Getenv("AUTOMUTEUS_PLAYING")
	if playingText == "" {
		playingText = os.Getenv("AUTOMUTEUS_LISTENING") // 後方互換
	}
	if playingText == "" {
		playingText = "エンジョブ村オリジナルBOT"
	}

	// pretty sure this needs to happen per-shard
	status := discordgo.UpdateStatusData{
		IdleSince: nil,
		Activities: []*discordgo.Activity{
			{
				Name: playingText,
				Type: discordgo.ActivityTypeGame, // ✅ ここが「プレイ中」
			},
		},
		AFK:    false,
		Status: "online", // online / idle / dnd / invisible / "" でも可
	}

	if err := dg.UpdateStatusComplex(status); err != nil {
		log.Println("failed to set playing status:", err)
	}

	if topGGToken != "" {
		dblClient, err := dbl.NewClient(topGGToken)
		if err != nil {
			log.Println("Error creating Top.gg client: ", err)
		}
		bot.TopGGClient = dblClient
	} else {
		log.Println("No TOP_GG_TOKEN provided")
	}

	return &bot
}

func (bot *Bot) InitTokenProvider(tp *tokenprovider.TokenProvider) {
	tp.Init(bot.RedisInterface.client, bot.PrimarySession)
}

func (bot *Bot) StartMetricsServer(nodeID string) error {
	return server.PrometheusMetricsServer(bot.RedisInterface.client, nodeID, "2112")
}

func (bot *Bot) Close() {
	// Ask every active game worker to unmute users and clean up before the
	// Discord session is closed. This reduces the chance of users remaining
	// server-muted during Docker restarts or image updates.
	activeWorkers := bot.signalAllEndGames()
	if activeWorkers > 0 {
		done := make(chan struct{})
		go func() {
			bot.SubscriberWG.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			log.Printf("Timed out waiting for %d active game worker(s) to stop cleanly", activeWorkers)
		}
	}

	bot.PrimarySession.Close()
	bot.RedisInterface.Close()
	bot.StorageInterface.Close()
}

// startGameSubscriber tracks the worker so shutdown can wait for its final
// unmute/cleanup sequence before closing the Discord session.
func (bot *Bot) startGameSubscriber(guildID, connectCode string, endGameChannel chan EndGameMessage) {
	bot.SubscriberWG.Add(1)
	go func() {
		defer bot.SubscriberWG.Done()
		bot.SubscribeToGameByConnectCode(guildID, connectCode, endGameChannel)
	}()
}

// signalAllEndGames is used during graceful shutdown. It does not close the
// channels because a sender/receiver may still be using them; it sends one
// buffered stop signal to each worker instead.
func (bot *Bot) signalAllEndGames() int {
	bot.ChannelsMapLock.Lock()
	channels := make([]chan EndGameMessage, 0, len(bot.EndGameChannels))
	for connectCode, ch := range bot.EndGameChannels {
		if ch != nil {
			channels = append(channels, ch)
		}
		delete(bot.EndGameChannels, connectCode)
	}
	bot.ChannelsMapLock.Unlock()

	for _, ch := range channels {
		select {
		case ch <- true:
		default:
		}
	}
	return len(channels)
}

// registerEndGameChannel stores the worker channel safely.
// A buffered channel prevents /stop from blocking if the subscriber is busy.
func (bot *Bot) registerEndGameChannel(connectCode string, endGameChannel chan EndGameMessage) {
	if connectCode == "" || endGameChannel == nil {
		return
	}

	bot.ChannelsMapLock.Lock()
	if bot.EndGameChannels == nil {
		bot.EndGameChannels = make(map[string]chan EndGameMessage)
	}
	previous, existed := bot.EndGameChannels[connectCode]
	bot.EndGameChannels[connectCode] = endGameChannel
	bot.ChannelsMapLock.Unlock()

	// If a stale worker existed for the same code, ask it to stop without blocking.
	if existed && previous != nil && previous != endGameChannel {
		select {
		case previous <- true:
		default:
		}
	}
}

// signalEndGame removes a worker channel from the map and signals it once.
// The operation is non-blocking and safe when /stop is pressed more than once.
func (bot *Bot) signalEndGame(connectCode string) bool {
	if connectCode == "" {
		return false
	}

	bot.ChannelsMapLock.Lock()
	endGameChannel, ok := bot.EndGameChannels[connectCode]
	if ok {
		delete(bot.EndGameChannels, connectCode)
	}
	bot.ChannelsMapLock.Unlock()

	if !ok || endGameChannel == nil {
		return false
	}

	select {
	case endGameChannel <- true:
	default:
	}
	return true
}

// removeEndGameChannel removes only the channel owned by the current worker.
// This avoids an older worker deleting a newer worker registered for the same code.
func (bot *Bot) removeEndGameChannel(connectCode string, endGameChannel chan EndGameMessage) {
	bot.ChannelsMapLock.Lock()
	if current, ok := bot.EndGameChannels[connectCode]; ok && current == endGameChannel {
		delete(bot.EndGameChannels, connectCode)
	}
	bot.ChannelsMapLock.Unlock()
}

var EmojiLock = sync.Mutex{}

func (bot *Bot) newGuild(emojiGuildID string) func(s *discordgo.Session, m *discordgo.GuildCreate) {
	return func(s *discordgo.Session, m *discordgo.GuildCreate) {
		gid, err := strconv.ParseUint(m.Guild.ID, 10, 64)
		if err != nil {
			log.Println(err)
		}

		go func() {
			_, err = bot.PostgresInterface.EnsureGuildExists(gid, m.Guild.Name)
			if err != nil {
				log.Println(err)
			}
		}()

		log.Printf("Added to new Guild, id %s, name %s", m.Guild.ID, m.Guild.Name)
		bot.RedisInterface.AddUniqueGuildCounter(m.Guild.ID)

		if emojiGuildID == "" {
			log.Println("[This is not an error] No explicit guildID provided for emojis; using the current guild default")
			emojiGuildID = m.Guild.ID
		}
		// only check/add emojis to the server denoted for emojis, OR, this server that we picked as a fallback above ^
		uploadMissingEmojis := emojiGuildID == m.Guild.ID

		EmojiLock.Lock()
		// only add the emojis if they haven't been added already. Saves api calls for bots in guilds
		if bot.StatusEmojis.isEmpty() {
			allEmojis, err := s.GuildEmojis(emojiGuildID)
			if err != nil {
				log.Println(err)
			} else {
				bot.verifyEmojis(s, emojiGuildID, true, allEmojis, uploadMissingEmojis)
				bot.verifyEmojis(s, emojiGuildID, false, allEmojis, uploadMissingEmojis)
			}
		}
		EmojiLock.Unlock()

		games := bot.RedisInterface.LoadAllActiveGames(m.Guild.ID)

		for _, connCode := range games {
			gsr := GameStateRequest{
				GuildID:     m.Guild.ID,
				ConnectCode: connCode,
			}
			lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(gsr, 10)
			if lock == nil {
				log.Printf("Unable to restore game %s for guild %s because the Redis lock could not be obtained", connCode, gsr.GuildID)
				continue
			}
			if dgs != nil && dgs.ConnectCode != "" {
				log.Println("Resubscribing to Redis events for an old game: " + connCode)
				killChan := make(chan EndGameMessage, 1)
				bot.startGameSubscriber(gsr.GuildID, dgs.ConnectCode, killChan)
				dgs.Subscribed = true

				bot.RedisInterface.SetDiscordGameState(dgs, lock)
				bot.registerEndGameChannel(dgs.ConnectCode, killChan)
			} else {
				bot.RedisInterface.SetDiscordGameState(nil, lock)
			}
		}
	}
}

func (bot *Bot) leaveGuild(_ *discordgo.Session, m *discordgo.GuildDelete) {
	log.Println("Bot was removed from Guild " + m.ID)
	bot.RedisInterface.LeaveUniqueGuildCounter(m.ID)

	err := bot.StorageInterface.DeleteGuildSettings(m.ID)
	if err != nil {
		log.Println(err)
	}
}

func (bot *Bot) forceEndGame(gsr GameStateRequest) {
	// Lock because we don't want anyone else modifying while we delete.
	// Do not spin forever if Redis is unhealthy; fail safely and log the problem.
	lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(gsr, 10)
	if lock == nil {
		log.Printf("Unable to force-end game for guild %s / code %s because the Redis lock could not be obtained", gsr.GuildID, gsr.ConnectCode)
		return
	}
	if dgs == nil {
		bot.RedisInterface.SetDiscordGameState(nil, lock)
		return
	}

	deleted := dgs.DeleteGameStateMsg(bot.PrimarySession, true)
	if deleted {
		go server.RecordDiscordRequests(bot.RedisInterface.client, server.MessageCreateDelete, 1)
	}

	bot.RedisInterface.SetDiscordGameState(dgs, lock)

	bot.RedisInterface.RemoveOldGame(dgs.GuildID, dgs.ConnectCode)

	// Note, this shouldn't be necessary with the TTL of the keys, but it can't hurt to clean up...
	bot.RedisInterface.DeleteDiscordGameState(dgs)
}

func MessageDeleteWorker(s *discordgo.Session, msgChannelID, msgID string, waitDur time.Duration) {
	log.Printf("Message worker is sleeping for %s before deleting message", waitDur.String())
	time.Sleep(waitDur)
	err := s.ChannelMessageDelete(msgChannelID, msgID)
	if err != nil {
		log.Println(err)
	}
}

func (bot *Bot) RefreshGameStateMessage(gsr GameStateRequest, sett *settings.GuildSettings) bool {
	lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(gsr, 10)
	if lock == nil || dgs == nil {
		log.Printf("Unable to refresh game message for guild %s because the Redis state lock could not be obtained", gsr.GuildID)
		return false
	}

	// don't try to edit this message, because we're about to delete it
	RemovePendingDGSEdit(dgs.GameStateMsg.MessageID)

	// note, this checks the variables being set, not whether or not the actual Discord message still exists
	gameExists := dgs.GameStateMsg.Exists()
	if !gameExists {
		// Always release the Redis lock, even when there is no active message.
		bot.RedisInterface.SetDiscordGameState(nil, lock)
		return false // no-op; no active game to refresh
	}

	deleted := dgs.DeleteGameStateMsg(bot.PrimarySession, false) // delete the old message
	created := dgs.CreateMessage(bot.PrimarySession, bot.gameStateResponse(dgs, sett), dgs.GameStateMsg.MessageChannelID, dgs.GameStateMsg.LeaderID)

	if deleted && created {
		go server.RecordDiscordRequests(bot.RedisInterface.client, server.MessageCreateDelete, 2)
	} else if deleted || created {
		go server.RecordDiscordRequests(bot.RedisInterface.client, server.MessageCreateDelete, 1)
	}

	bot.RedisInterface.SetDiscordGameState(dgs, lock)
	// if for whatever reason the message failed to create, this would catch it
	return dgs.GameStateMsg.Exists()
}

func (bot *Bot) getInfo() command.BotInfo {
	totalGuilds := rediskey.GetGuildCounter(context.Background(), bot.RedisInterface.client)
	activeGames := rediskey.GetActiveGames(context.Background(), bot.RedisInterface.client, GameTimeoutSeconds)

	totalUsers := rediskey.GetTotalUsers(context.Background(), bot.RedisInterface.client)
	if totalUsers == rediskey.NotFound {
		totalUsers = rediskey.RefreshTotalUsers(context.Background(), bot.RedisInterface.client, bot.PostgresInterface.Pool)
	}

	totalGames := rediskey.GetTotalGames(context.Background(), bot.RedisInterface.client)
	if totalGames == rediskey.NotFound {
		totalGames = rediskey.RefreshTotalGames(context.Background(), bot.RedisInterface.client, bot.PostgresInterface.Pool)
	}
	return command.BotInfo{
		Version:     bot.version,
		Commit:      bot.commit,
		ShardID:     bot.PrimarySession.ShardID,
		ShardCount:  bot.PrimarySession.ShardCount,
		TotalGuilds: totalGuilds,
		ActiveGames: activeGames,
		TotalUsers:  totalUsers,
		TotalGames:  totalGames,
	}
}

func linkPlayer(redis *RedisInterface, dgs *GameState, userID, color string) (command.LinkStatus, error) {
	var auData amongus.PlayerData
	found := false
	if game.IsColorString(color) {
		auData, found = dgs.GameData.GetByColor(color)
	}
	if found {
		foundID := dgs.AttemptPairingByUserIDs(auData, map[string]interface{}{userID: struct{}{}})
		if foundID != "" {
			err := redis.AddUsernameLink(dgs.GuildID, userID, auData.Name)
			if err != nil {
				log.Println(err)
			}
			return command.LinkSuccess, nil
		} else {
			err := fmt.Sprintf("No player in the current game was found matching %s", discord.MentionByUserID(userID))
			return command.LinkNoPlayer, errors.New(err)
		}
	} else {
		err := fmt.Errorf("no game data found for player %s and color %s", discord.MentionByUserID(userID), color)
		return command.LinkNoGameData, err
	}
}

func unlinkPlayer(dgs *GameState, userID string) command.UnlinkStatus {
	// if we found the player and cleared their data
	success := dgs.ClearPlayerData(userID)
	if success {
		return command.UnlinkSuccess
	} else {
		return command.UnlinkNoPlayer
	}
}

func getTrackingChannel(guild *discordgo.Guild, userID string) string {
	// loop over all the channels in the discord and cross-reference with the one that the .au new author is in
	for _, v := range guild.VoiceStates {
		// if the User who typed au new is in a voice channel
		if v.UserID == userID {
			return v.ChannelID
		}
	}
	return ""
}

func (bot *Bot) newGame(dgs *GameState) (_ command.NewStatus, activeGames int64) {
	if dgs.GameStateMsg.Exists() {
		bot.signalEndGame(dgs.ConnectCode)
		dgs.Reset()
	} else {
		premStatus, days, err := bot.PostgresInterface.GetGuildOrUserPremiumStatus(
			bot.official, bot.TopGGClient, dgs.GuildID, dgs.GameStateMsg.LeaderID)
		if err != nil {
			log.Println("Error in /newgame get premium:", err)
		}
		premTier := premium.FreeTier
		if !premium.IsExpired(premStatus, days) {
			premTier = premStatus
		}

		// Premium users should always be allowed to start new games; only check the free guilds
		if premTier == premium.FreeTier {
			activeGames = rediskey.GetActiveGames(context.Background(), bot.RedisInterface.client, GameTimeoutSeconds)
			if activeGames > command.DefaultMaxActiveGames {
				return command.NewLockout, activeGames
			}
		}
	}

	dgs.ConnectCode = generateConnectCode(dgs.GuildID)
	dgs.Subscribed = true

	return command.NewSuccess, activeGames
}
