package bot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/automuteus/automuteus/v8/internal/server"
	"github.com/automuteus/automuteus/v8/pkg/amongus"
	"github.com/automuteus/automuteus/v8/pkg/discord"
	"github.com/automuteus/automuteus/v8/pkg/game"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/automuteus/automuteus/v8/pkg/storage"
	"github.com/automuteus/automuteus/v8/pkg/task"
	"github.com/go-redis/redis/v8"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"log"
	"strconv"
	"strings"
	"time"
)

type EndGameMessage bool

func resetCaptureTimer(timer *time.Timer, timeout time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(timeout)
}

// failSafeEndGame always tries to unmute participants before removing the game state.
// This prevents users from remaining server-muted when Capture, Redis Pub/Sub, or the
// subscriber worker stops unexpectedly.
func (bot *Bot) failSafeEndGame(dgsRequest GameStateRequest, reason string) {
	dgs := bot.RedisInterface.GetReadOnlyDiscordGameState(dgsRequest)
	if dgs != nil {
		var unmuteErr error
		for attempt := 1; attempt <= 3; attempt++ {
			unmuteErr = bot.applyToAll(dgs, false, false)
			if unmuteErr == nil {
				break
			}
			log.Printf("Unmute attempt %d/3 failed while ending game (%s): %v", attempt, reason, unmuteErr)
			if attempt < 3 {
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			}
		}
		if unmuteErr != nil {
			log.Printf("Unable to unmute every user after 3 attempts while ending game (%s): %v", reason, unmuteErr)
		}
	}
	bot.forceEndGame(dgsRequest)
}

func (bot *Bot) SubscribeToGameByConnectCode(guildID, connectCode string, endGameChannel chan EndGameMessage) {
	log.Println("Started Redis Subscription worker for " + connectCode)

	notify := task.Subscribe(ctx, bot.RedisInterface.client, connectCode)
	notifyChannel := notify.Channel()
	defer func() {
		if err := notify.Close(); err != nil {
			log.Println(err)
		}
	}()

	captureTimeout := time.Second * time.Duration(bot.captureTimeout)
	timer := time.NewTimer(captureTimeout)
	defer timer.Stop()

	dgsRequest := GameStateRequest{
		GuildID:     guildID,
		ConnectCode: connectCode,
	}

	// indicate to the broker that we're online and ready to start processing messages
	task.Ack(ctx, bot.RedisInterface.client, connectCode)

	for {
		select {
		case message, ok := <-notifyChannel:
			if !ok || message == nil {
				log.Printf("Redis Pub/Sub channel closed for game %s; ending the game safely", connectCode)
				bot.removeEndGameChannel(connectCode, endGameChannel)
				bot.failSafeEndGame(dgsRequest, "Redis Pub/Sub channel closed")
				return
			}
			resetCaptureTimer(timer, captureTimeout)

			// anytime we get a notification message, continue pulling messages off the list until there are no more
			for {
				job, err := task.PopJob(ctx, bot.RedisInterface.client, connectCode)
				if errors.Is(err, redis.Nil) {
					break
				} else if err != nil {
					log.Println(err)
					break
				}
				payload, ok := job.Payload.(string)
				if !ok {
					log.Printf("Ignoring job type %d for game %s because its payload was not a string", job.JobType, connectCode)
					continue
				}
				log.Printf("Popped job of type %d w/ payload %s\n", job.JobType, payload)
				bot.refreshGameLiveness(connectCode)
				bot.RedisInterface.RefreshActiveGame(guildID, connectCode)

				gameEvent := storage.PostgresGameEvent{
					GameID:    -1,
					UserID:    nil,
					EventTime: int32(time.Now().Unix()),
					EventType: int16(job.JobType),
					Payload:   payload,
				}
				correlatedUserID := ""
				sett := bot.StorageInterface.GetGuildSettings(guildID)

				switch job.JobType {

				// ======================================================
				// ★ ConnectionJob = Capture の接続/切断通知
				// ======================================================
				case task.ConnectionJob:
					lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(dgsRequest, 10)
					if lock == nil || dgs == nil {
						log.Printf("Unable to process Capture connection state for game %s because the Redis lock could not be obtained", connectCode)
						break
					}

					// 変更前の接続状態を保持（変化があったときだけ Refresh）
					prevCapture := dgs.CaptureConnected

					if payload == "true" {
						dgs.Linked = true

						// ★ Capture 接続確立！
						dgs.CaptureConnected = true
						dgs.LastCapturePing = time.Now().Unix()
					} else {
						dgs.Linked = false

						// ★ Capture 切断
						dgs.CaptureConnected = false
						dgs.LastCapturePing = time.Now().Unix()
					}

					dgs.ConnectCode = connectCode
					bot.RedisInterface.SetDiscordGameState(dgs, lock)

					bot.handleTrackedMembers(bot.PrimarySession, sett, 0, NoPriority, dgsRequest)

					// ★ 接続状態が変化した瞬間だけ「作り直し」
					//   - false -> true ならボタン出現
					//   - true -> false ならボタン消える（任意だけど安全）
					if prevCapture != dgs.CaptureConnected {
						bot.RefreshGameStateMessage(dgsRequest, sett)
					} else {
						bot.DispatchRefreshOrEdit(dgs, dgsRequest, sett)
					}

				// ======================================================
				// ★ Lobby/State/Player Job でも
				//   「ConnectionJobが来ない保険」で CaptureConnected を true にする
				// ======================================================
				case task.LobbyJob:
					var lobby game.Lobby
					err = json.Unmarshal([]byte(payload), &lobby)
					if err != nil {
						log.Println(err)
						break
					}
					bot.processLobby(sett, lobby, dgsRequest)

				case task.StateJob:
					num, err := strconv.ParseInt(payload, 10, 64)
					if err != nil {
						log.Println(err)
						break
					}
					bot.processTransition(game.Phase(num), dgsRequest)

				case task.PlayerJob:
					var player game.Player
					err = json.Unmarshal([]byte(payload), &player)
					if err != nil {
						log.Println(err)
						break
					}
					if player.Color > 17 || player.Color < 0 {
						break
					}

					shouldHandleTracked, userID, readOnlyDgs, err := bot.processPlayer(sett, player, dgsRequest)
					if shouldHandleTracked {
						bot.handleTrackedMembers(bot.PrimarySession, sett, 0, NoPriority, dgsRequest)
					}
					if err != nil {
						log.Printf("Error while processing player event for game %s: %v", connectCode, err)
						if readOnlyDgs != nil && readOnlyDgs.GameStateMsg.MessageChannelID != "" {
							_, sendErr := bot.PrimarySession.ChannelMessageSend(readOnlyDgs.GameStateMsg.MessageChannelID, sett.LocalizeMessage(&i18n.Message{
								ID:    "processplayer.error",
								Other: "{{.User}} のミュート処理中にエラーが発生しました。{{.VoiceChannel}} でメンバーをミュート／スピーカーミュートする権限がBOTにあるか確認してください。",
							},
								map[string]interface{}{
									"User":         discord.MentionByUserID(userID),
									"VoiceChannel": discord.MentionByChannelID(readOnlyDgs.VoiceChannel),
								},
							))
							if sendErr != nil {
								log.Printf("Unable to send player-processing error to Discord: %v", sendErr)
							} else {
								server.RecordDiscordRequests(bot.RedisInterface.client, server.MessageCreateDelete, 1)
							}
						}
					}
					correlatedUserID = userID

				case task.GameOverJob:
					var gameOverResult game.Gameover
					err := json.Unmarshal([]byte(payload), &gameOverResult)
					if err != nil {
						log.Println(err)
						break
					}

					// we only need a read-only state for making the game summary message
					dgs := bot.RedisInterface.GetReadOnlyDiscordGameState(dgsRequest)
					if dgs != nil {
						delTime := sett.GetDeleteGameSummaryMinutes()
						if delTime != 0 {
							winners := getWinners(*dgs, gameOverResult)
							buf := bytes.NewBuffer([]byte{})
							for i, v := range winners {
								roleStr := "クルーメイト"
								if v.role == game.ImposterRole {
									roleStr = "インポスター"
								}
								buf.WriteString(fmt.Sprintf("<@%s>", v.userID))
								if i < len(winners)-1 {
									buf.WriteRune('、')
								} else {
									buf.WriteString(fmt.Sprintf(" が%sとして勝利しました。", roleStr))
								}
							}
							embed := gameOverMessage(dgs, bot.StatusEmojis, sett, buf.String())
							channelID := dgs.GameStateMsg.MessageChannelID
							if sett.GetMatchSummaryChannelID() != "" {
								channelID = sett.GetMatchSummaryChannelID()
							}
							msg, err := bot.PrimarySession.ChannelMessageSendEmbed(channelID, embed)
							if delTime > 0 && err == nil {
								server.RecordDiscordRequests(bot.RedisInterface.client, server.MessageCreateDelete, 2)
								go MessageDeleteWorker(bot.PrimarySession, msg.ChannelID, msg.ID, time.Minute*time.Duration(delTime))
							} else if err == nil {
								server.RecordDiscordRequests(bot.RedisInterface.client, server.MessageCreateDelete, 1)
							}
						}
						go dumpGameToPostgres(*dgs, bot.PostgresInterface, gameOverResult)

						// refresh the game message if the setting is marked
						if sett.AutoRefresh {
							bot.RefreshGameStateMessage(dgsRequest, sett)
						}

						lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(dgsRequest, 10)
						if lock == nil || dgs == nil {
							log.Printf("Unable to reset match information for game %s because the Redis lock could not be obtained", connectCode)
							break
						}
						dgs.MatchID = -1
						dgs.MatchStartUnix = -1
						bot.RedisInterface.SetDiscordGameState(dgs, lock)
					}
				}

				if job.JobType != task.ConnectionJob {
					go func(userID string, ge storage.PostgresGameEvent) {
						dgs := bot.RedisInterface.GetReadOnlyDiscordGameState(dgsRequest)
						if dgs != nil && dgs.MatchID > 0 && dgs.MatchStartUnix > 0 {
							ge.GameID = dgs.MatchID
							if userID != "" {
								num, err := strconv.ParseUint(userID, 10, 64)
								if err != nil {
									log.Println(err)
									ge.UserID = nil
								} else {
									ge.UserID = &num
								}
								log.Printf("Adding postgres event with user id %d\n", ge.UserID)
							}

							err := bot.PostgresInterface.AddEvent(&ge)
							if err != nil {
								log.Println(err)
							}
						}
					}(correlatedUserID, gameEvent)
				}
			}

		case <-timer.C:
			log.Printf("Ending game w/ code %s after %d seconds of Capture inactivity\n", connectCode, bot.captureTimeout)
			bot.removeEndGameChannel(connectCode, endGameChannel)
			bot.failSafeEndGame(dgsRequest, "Capture timeout")
			return
		case <-endGameChannel:
			log.Println("Redis subscriber received kill signal, ending the game safely")
			bot.removeEndGameChannel(connectCode, endGameChannel)
			bot.failSafeEndGame(dgsRequest, "manual stop or game replacement")
			return
		}
	}
}

type winnerRecord struct {
	userID string
	role   game.GameRole
}

func getWinners(dgs GameState, gameOver game.Gameover) []winnerRecord {
	var winners []winnerRecord

	imposterWin := gameOver.GameOverReason == game.ImpostorByKill ||
		gameOver.GameOverReason == game.ImpostorByVote ||
		gameOver.GameOverReason == game.ImpostorBySabotage ||
		gameOver.GameOverReason == game.ImpostorDisconnect

	for _, player := range dgs.UserData {
		if player.GetPlayerName() != amongus.UnlinkedPlayerName {
			for _, v := range gameOver.PlayerInfos {
				// only override for the imposters
				if player.GetPlayerName() == v.Name {
					if (v.IsImpostor && imposterWin) || (!v.IsImpostor && !imposterWin) {
						role := game.CrewmateRole
						if v.IsImpostor {
							role = game.ImposterRole
						}
						winners = append(winners, winnerRecord{
							userID: player.User.UserID,
							role:   role,
						})
					}
				}
			}
		}
	}
	return winners
}

func finalizePlayerStateUpdate(
	saveState func(),
	refreshMessage func(),
	dispatchMessage func(),
	initialConnect bool,
	dispatchRequested bool,
) {
	if saveState != nil {
		saveState()
	}
	if initialConnect {
		if refreshMessage != nil {
			refreshMessage()
		}
		return
	}
	if dispatchRequested && dispatchMessage != nil {
		dispatchMessage()
	}
}

func (bot *Bot) processPlayer(sett *settings.GuildSettings, player game.Player, dgsRequest GameStateRequest) (bool, string, *GameState, error) {
	var err error
	if player.Name != "" {
		lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(dgsRequest, 10)
		if lock == nil || dgs == nil {
			return false, "", nil, errors.New("could not obtain Redis game-state lock while processing player data")
		}
		dgs.Linked = true

		// ★ 追加: ConnectionJobが来ない場合の保険
		initialConnect := false
		if !dgs.CaptureConnected {
			dgs.CaptureConnected = true
			dgs.LastCapturePing = time.Now().Unix()
			initialConnect = true
		} else {
			dgs.LastCapturePing = time.Now().Unix()
		}

		// Save and release the Redis lock before refreshing or editing the Discord message.
		// RefreshGameStateMessage obtains the same game-state lock, so invoking it while
		// this lock is held would cause the refresh to fail.
		dispatchAfterSave := false
		defer func() {
			finalizePlayerStateUpdate(
				func() {
					bot.RedisInterface.SetDiscordGameState(dgs, lock)
				},
				func() {
					go bot.RefreshGameStateMessage(dgsRequest, sett)
				},
				func() {
					bot.DispatchRefreshOrEdit(dgs, dgsRequest, sett)
				},
				initialConnect,
				dispatchAfterSave,
			)
		}()

		if player.Disconnected || player.Action == game.LEFT {
			if player.Disconnected {
				log.Println("I detected that " + player.Name + " disconnected, I'm purging their player data!")
				dgs.ClearPlayerDataByPlayerName(player.Name)
			}
			_, _, data := dgs.GameData.UpdatePlayer(player)

			userID := dgs.AttemptPairingByMatchingNames(data)
			// try pairing via the cached usernames
			if userID == "" {
				var uids map[string]interface{}
				uids, err = bot.RedisInterface.GetUsernameOrUserIDMappings(dgs.GuildID, player.Name)
				userID = dgs.AttemptPairingByUserIDs(data, uids)
			} else {
				err = bot.applyToSingle(dgs, userID, false, false)
			}

			dgs.GameData.ClearPlayerData(player.Name)

			// only update the message if we're not in the tasks phase (info leaks)
			if dgs.GameData.GetPhase() != game.TASKS {
				dispatchAfterSave = true
			}

			return true, userID, dgs, err
		}
		updated, isAliveUpdated, data := dgs.GameData.UpdatePlayer(player)
		switch {
		case player.Action == game.JOINED:
			log.Println("Detected a player joined, refreshing User data mappings")
			userID := dgs.AttemptPairingByMatchingNames(data)
			if userID == "" {
				var uids map[string]interface{}
				uids, err = bot.RedisInterface.GetUsernameOrUserIDMappings(dgs.GuildID, player.Name)
				userID = dgs.AttemptPairingByUserIDs(data, uids)
			}
			dispatchAfterSave = true
			return true, userID, dgs, err
		case updated:
			userID := dgs.AttemptPairingByMatchingNames(data)
			if userID == "" {
				var uids map[string]interface{}
				uids, err = bot.RedisInterface.GetUsernameOrUserIDMappings(dgs.GuildID, player.Name)
				userID = dgs.AttemptPairingByUserIDs(data, uids)
			}
			if isAliveUpdated && dgs.GameData.GetPhase() == game.TASKS {
				if sett.GetUnmuteDeadDuringTasks() || player.Action == game.EXILED {
					dispatchAfterSave = true
					return true, userID, dgs, err
				}
				log.Println("NOT updating the discord status message; would leak info")
				return false, userID, dgs, err
			}
			dispatchAfterSave = true
			if player.Action == game.EXILED {
				return false, userID, dgs, err // don't apply a mute to this player
			}
			return true, userID, dgs, err
		default:
			return false, "", nil, nil
		}
	}
	return false, "", nil, nil
}

func (bot *Bot) processTransition(phase game.Phase, dgsRequest GameStateRequest) {
	sett := bot.StorageInterface.GetGuildSettings(dgsRequest.GuildID)
	lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(dgsRequest, 10)
	if lock == nil || dgs == nil {
		log.Printf("Unable to process phase transition for guild %s because the Redis lock could not be obtained", dgsRequest.GuildID)
		return
	}

	// ★ 追加: ConnectionJobが来ない場合の保険（Transition来た=Capture接続済み）
	initialConnect := false
	if !dgs.CaptureConnected {
		dgs.CaptureConnected = true
		dgs.LastCapturePing = time.Now().Unix()
		initialConnect = true
	} else {
		dgs.LastCapturePing = time.Now().Unix()
	}

	oldPhase := dgs.GameData.UpdatePhase(phase)
	if oldPhase == phase {
		// Persist CaptureConnected/LastCapturePing and always release the lock.
		dgs.Linked = true
		bot.RedisInterface.SetDiscordGameState(dgs, lock)
		if initialConnect {
			bot.RefreshGameStateMessage(dgsRequest, sett)
		}
		return
	}
	dgs.Linked = true
	// if we started a new game
	if oldPhase == game.LOBBY && phase == game.TASKS {
		matchStart := time.Now().Unix()
		dgs.MatchStartUnix = matchStart
		gameID := startGameInPostgres(*dgs, bot.PostgresInterface)
		dgs.MatchID = int64(gameID)
		log.Printf("New match has begun. ID %d and starttime %d\n", gameID, matchStart)
	}

	bot.RedisInterface.SetDiscordGameState(dgs, lock)

	// ★ 初回接続ならここで1回 Refresh（ボタン付与）
	if initialConnect {
		bot.RefreshGameStateMessage(dgsRequest, sett)
	}

	switch phase {
	case game.MENU:
		bot.DispatchRefreshOrEdit(dgs, dgsRequest, sett)
		err := bot.applyToAll(dgs, false, false)
		if err != nil {
			log.Println("Error in unmuting all users when returning to menu ", err)
		}
	case game.GAMEOVER:
		phase = game.LOBBY
		fallthrough
	case game.LOBBY:
		delay := sett.Delays.GetDelay(oldPhase, phase)
		bot.handleTrackedMembers(bot.PrimarySession, sett, delay, NoPriority, dgsRequest)

		bot.DispatchRefreshOrEdit(dgs, dgsRequest, sett)

	case game.TASKS:
		delay := sett.Delays.GetDelay(oldPhase, phase)
		priority := AlivePriority
		if oldPhase == game.LOBBY {
			priority = NoPriority
		}

		bot.handleTrackedMembers(bot.PrimarySession, sett, delay, priority, dgsRequest)
		bot.DispatchRefreshOrEdit(dgs, dgsRequest, sett)

	case game.DISCUSS:
		delay := sett.Delays.GetDelay(oldPhase, phase)
		bot.handleTrackedMembers(bot.PrimarySession, sett, delay, DeadPriority, dgsRequest)

		if sett.AutoRefresh {
			bot.RefreshGameStateMessage(dgsRequest, sett)
		} else {
			bot.DispatchRefreshOrEdit(dgs, dgsRequest, sett)
		}
	}
}

func (bot *Bot) processLobby(sett *settings.GuildSettings, lobby game.Lobby, dgsRequest GameStateRequest) {
	lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(dgsRequest, 10)
	if lock == nil || dgs == nil {
		log.Printf("Unable to process lobby data for guild %s because the Redis lock could not be obtained", dgsRequest.GuildID)
		return
	}

	// ★ 追加: Lobby来た=Capture接続済みの保険
	initialConnect := false
	if !dgs.CaptureConnected {
		dgs.CaptureConnected = true
		dgs.LastCapturePing = time.Now().Unix()
		initialConnect = true
	} else {
		dgs.LastCapturePing = time.Now().Unix()
	}

	dgs.GameData.SetRoomRegionMap(lobby.LobbyCode, lobby.Region.ToString(), lobby.PlayMap)
	bot.RedisInterface.SetDiscordGameState(dgs, lock)

	// ★ 初回接続なら Refresh（ボタン付与）
	if initialConnect {
		bot.RefreshGameStateMessage(dgsRequest, sett)
	} else {
		bot.DispatchRefreshOrEdit(dgs, dgsRequest, sett)
	}
}

func startGameInPostgres(dgs GameState, psql *storage.PsqlInterface) uint64 {
	if dgs.MatchStartUnix < 0 {
		return 0
	}
	gid, err := strconv.ParseUint(dgs.GuildID, 10, 64)
	if err != nil {
		log.Println(err)
		return 0
	}
	pgame := &storage.PostgresGame{
		GameID:      -1,
		GuildID:     gid,
		ConnectCode: dgs.ConnectCode,
		StartTime:   int32(dgs.MatchStartUnix),
		WinType:     -1,
		EndTime:     -1,
	}
	i, err := psql.AddInitialGame(pgame)
	if err != nil {
		log.Println(err)
	}
	return i
}

func dumpGameToPostgres(dgs GameState, psql *storage.PsqlInterface, gameOver game.Gameover) {
	if dgs.MatchID < 0 || dgs.MatchStartUnix < 0 {
		log.Println("dgs match id or start time is <0; not dumping game to Postgres")
		return
	}
	end := time.Now().Unix()

	userGames := make([]*storage.PostgresUserGame, 0)

	imposterWin := gameOver.GameOverReason == game.ImpostorByKill ||
		gameOver.GameOverReason == game.ImpostorBySabotage ||
		gameOver.GameOverReason == game.ImpostorByVote ||
		gameOver.GameOverReason == game.ImpostorDisconnect

	for _, v := range dgs.UserData {
		if v.GetPlayerName() != amongus.UnlinkedPlayerName {
			inGameData, found := dgs.GameData.GetByName(v.GetPlayerName())
			if !found {
				log.Println("No game data found for that player")
				continue
			}

			uid, err := strconv.ParseUint(v.User.UserID, 10, 64)
			if err != nil {
				log.Println(err)
				continue
			}
			gid, err := strconv.ParseUint(dgs.GuildID, 10, 64)
			if err != nil {
				log.Println(err)
				continue
			}

			puser, err := psql.EnsureUserExists(uid)
			if err != nil || puser == nil {
				log.Println(err)
				continue
			}

			won := !imposterWin
			role := game.CrewmateRole
			for _, pi := range gameOver.PlayerInfos {
				if pi.IsImpostor {
					if strings.EqualFold(pi.Name, inGameData.Name) {
						role = game.ImposterRole
						won = imposterWin
						break
					}
				}
			}

			userGames = append(userGames, &storage.PostgresUserGame{
				UserID:      puser.UserID,
				GuildID:     gid,
				GameID:      dgs.MatchID,
				PlayerName:  inGameData.Name,
				PlayerColor: int16(inGameData.Color),
				PlayerRole:  int16(role),
				PlayerWon:   won,
			})
		}
	}
	log.Printf("Game %d has been completed and recorded in postgres\n", dgs.MatchID)

	err := psql.UpdateGameAndPlayers(dgs.MatchID, int16(gameOver.GameOverReason), end, userGames)
	if err != nil {
		log.Println(err)
	}
}
