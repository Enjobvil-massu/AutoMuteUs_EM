package bot

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/automuteus/automuteus/v8/internal/server"
	"github.com/automuteus/automuteus/v8/pkg/storage"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/automuteus/automuteus/v8/bot/command"
	"github.com/automuteus/automuteus/v8/bot/setting"
	redis_common "github.com/automuteus/automuteus/v8/common"
	"github.com/automuteus/automuteus/v8/pkg/amongus"
	"github.com/automuteus/automuteus/v8/pkg/discord"
	"github.com/automuteus/automuteus/v8/pkg/premium"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

var MatchIDRegex = regexp.MustCompile(`^[A-Z0-9]{8}:[0-9]+$`)

var DownloadPermissions = []int64{
	discordgo.PermissionAttachFiles,
}

var RequiredPermissions = []int64{
	discordgo.PermissionViewChannel, discordgo.PermissionSendMessages,
	discordgo.PermissionManageMessages, discordgo.PermissionEmbedLinks,
	discordgo.PermissionUseExternalEmojis,
}

var VoicePermissions = []int64{
	discordgo.PermissionVoiceMuteMembers, discordgo.PermissionVoiceDeafenMembers,
}

const (
	resetUserConfirmedID          = "reset-user-confirmed"
	resetUserCanceledID           = "reset-user-canceled"
	resetGuildConfirmedID         = "reset-guild-confirmed"
	resetGuildCanceledID          = "reset-guild-canceled"
	downloadGuildConfirmedID      = "download-guild-confirmed"
	downloadUsersConfirmedID      = "download-users-confirmed"
	downloadUsersGamesConfirmedID = "download-users-games-confirmed"
	downloadGamesConfirmedID      = "download-games-confirmed"
	downloadGameEventsConfirmedID = "download-game-events-confirmed"
	downloadCanceledID            = "download-canceled"

	// ===== 追加: /stop ボタン用 =====
	// CustomID: "stop-game:<starterUserID>:<connectCode>"
	stopButtonIDPrefix = "stop-game"

	// ===== 追加: /link ボタン用 =====
	// CustomID: "link-game:<starterUserID>:<connectCode>"
	linkButtonIDPrefix = "link-game"

	// ===== 追加: 表示更新ボタン用 =====
	// CustomID: "refresh-game:<starterUserID>:<connectCode>"
	refreshButtonIDPrefix = "refresh-game"

	// ===== 追加: /link ユーザー選択用 =====
	// CustomID: "link-select-user:<starterUserID>:<connectCode>"
	linkUserSelectPrefix = "link-select-user"

	// ===== 追加: /link 色選択ボタン用 =====
	// CustomID: "link-color:<starterUserID>:<targetUserID>:<connectCode>:<ColorName>"
	linkColorButtonPrefix = "link-color"
)

const staleStartControlMessage = "この操作画面は古いため使用できません。現在実行中の /start 画面から操作してください。"

// isCurrentStartControlGame prevents buttons left behind by a completed or
// replaced /start session from controlling a newer game in the same channel.
func isCurrentStartControlGame(dgs *GameState, connectCode string) bool {
	if dgs == nil {
		return false
	}
	expected := strings.TrimSpace(connectCode)
	current := strings.TrimSpace(dgs.ConnectCode)
	return expected != "" && current != "" && strings.EqualFold(current, expected)
}

// ===== 追加: /new(/start) の操作ボタン =====
func stopButtonComponents(starterUserID, connectCode string, sett *settings.GuildSettings) []discordgo.MessageComponent {
	labelLink := "ホストによる手動リンク"
	labelRefresh := "更新"
	labelStop := "停止"

	stopID := fmt.Sprintf("%s:%s:%s", stopButtonIDPrefix, starterUserID, connectCode)
	linkID := fmt.Sprintf("%s:%s:%s", linkButtonIDPrefix, starterUserID, connectCode)
	refreshID := fmt.Sprintf("%s:%s:%s", refreshButtonIDPrefix, starterUserID, connectCode)

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				// 左側: ホストによる手動リンク
				discordgo.Button{
					CustomID: linkID,
					Style:    discordgo.SuccessButton,
					Label:    labelLink,
					Emoji:    discordgo.ComponentEmoji{Name: "👉"},
				},
				// 中央: 公開ゲーム状態メッセージを作り直す
				discordgo.Button{
					CustomID: refreshID,
					Style:    discordgo.PrimaryButton,
					Label:    labelRefresh,
					Emoji:    discordgo.ComponentEmoji{Name: "🔄"},
				},
				// 右側: /stop ボタン
				discordgo.Button{
					CustomID: stopID,
					Style:    discordgo.DangerButton,
					Label:    labelStop,
					Emoji:    discordgo.ComponentEmoji{Name: "👉"},
				},
			},
		},
	}
}

// ===== 追加: /link 用 色ボタン生成ヘルパー =====
func linkColorButtons(starterUserID, targetUserID, connectCode string) []discordgo.MessageComponent {
	// CustomIDに渡す内部値は従来どおり英語のまま維持し、表示だけ日本語化します。
	colors := []string{
		"Red", "Blue", "Green", "Pink", "Orange",
		"Yellow", "Black", "White", "Purple", "Brown",
		"Cyan", "Lime", "Maroon", "Rose", "Banana",
		"Gray", "Tan", "Coral",
	}

	components := make([]discordgo.MessageComponent, 0, 4)
	row := discordgo.ActionsRow{}

	for i, color := range colors {
		colorEmoji := GlobalAlivenessEmojis[true][i]

		row.Components = append(row.Components, discordgo.Button{
			CustomID: fmt.Sprintf(
				"%s:%s:%s:%s:%s",
				linkColorButtonPrefix,
				starterUserID,
				targetUserID,
				connectCode,
				color,
			),
			Style: discordgo.PrimaryButton,
			Label: command.JapaneseColorName(color),
			Emoji: discordgo.ComponentEmoji{
				ID: colorEmoji.ID,
			},
		})

		if len(row.Components) == 5 {
			components = append(components, row)
			row = discordgo.ActionsRow{}
		}
	}

	row.Components = append(row.Components, discordgo.Button{
		CustomID: fmt.Sprintf(
			"%s:%s:%s:%s:%s",
			linkColorButtonPrefix,
			starterUserID,
			targetUserID,
			connectCode,
			"UNLINK",
		),
		Style: discordgo.SecondaryButton,
		Label: "リンク解除",
		Emoji: discordgo.ComponentEmoji{
			Name: X,
		},
	})

	components = append(components, row)
	return components
}

// isUserInTrackedVoiceChannel returns true only when the user is currently in
// the voice channel that /start registered for this game.
func isUserInTrackedVoiceChannel(g *discordgo.Guild, userID, trackedChannelID string) bool {
	if g == nil || userID == "" || trackedChannelID == "" {
		return false
	}

	for _, voiceState := range g.VoiceStates {
		if voiceState != nil && voiceState.UserID == userID && voiceState.ChannelID == trackedChannelID {
			return true
		}
	}
	return false
}

// isCurrentSelfLinkPanel rejects buttons left behind by an old or replaced
// game-state message without changing the public UI.
func isCurrentSelfLinkPanel(dgs *GameState, messageID, channelID string) bool {
	if dgs == nil || !dgs.GameStateMsg.Exists() || messageID == "" || channelID == "" {
		return false
	}
	return dgs.GameStateMsg.MessageID == messageID && dgs.GameStateMsg.MessageChannelID == channelID
}

// linkedPlayerNameForUser reports whether the Discord user is already linked.
func linkedPlayerNameForUser(dgs *GameState, userID string) (string, bool) {
	if dgs == nil || userID == "" {
		return "", false
	}

	userData, ok := dgs.UserData[userID]
	if !ok {
		return "", false
	}

	playerName := strings.TrimSpace(userData.InGameName)
	if playerName == "" || playerName == amongus.UnlinkedPlayerName {
		return "", false
	}
	return playerName, true
}

// linkedUserForPlayerName prevents two Discord users from claiming the same
// Among Us player/color. The caller already holds the Redis game-state lock.
func linkedUserForPlayerName(dgs *GameState, playerName, exceptUserID string) (string, bool) {
	if dgs == nil {
		return "", false
	}

	playerName = strings.TrimSpace(playerName)
	if playerName == "" || playerName == amongus.UnlinkedPlayerName {
		return "", false
	}

	for userID, userData := range dgs.UserData {
		if userID == exceptUserID {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(userData.InGameName), playerName) {
			return userID, true
		}
	}
	return "", false
}

func (bot *Bot) handleInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	respondChan := make(chan *discordgo.InteractionResponse, 1)
	ticker := time.NewTicker(time.Second * 2)
	defer ticker.Stop()

	var followUpMsg *discordgo.Message
	var err error

	// Get the result in the background. Recovering here prevents one malformed
	// interaction from leaving Discord's interaction worker waiting forever.
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("Recovered from interaction panic: %v", recovered)
				respondChan <- command.PrivateResponse("処理中にエラーが発生しました。少し待ってからもう一度お試しください。")
			}
		}()
		respondChan <- bot.slashCommandHandler(s, i)
	}()

	for {
		select {
		case <-ticker.C:
			// only followup the first time
			if followUpMsg == nil {
				err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Flags:   1 << 6,
						Content: "処理中です。少しだけお待ちください。",
					},
				})
				if err != nil {
					log.Println("err issuing wait response ", err)
				}
				followUpMsg, err = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
					Content: Hourglass,
				})
				if err != nil {
					log.Println("Error creating followup message: ", err)
				}
			}
			// don't return here

		case resp := <-respondChan:
			if followUpMsg != nil {
				if resp != nil && resp.Data != nil {
					content := resp.Data.Content
					if content == "" {
						content = "\u200b"
					}
					followUpMsg, err = s.FollowupMessageEdit(i.Interaction, followUpMsg.ID, &discordgo.WebhookEdit{
						Content:    &content,
						Components: &resp.Data.Components,
						Embeds:     &resp.Data.Embeds,
					})
				} else {
					//TODO if this shows up in logs regularly, print more context
					log.Println("received a nil response, or resp.data was nil")
				}
			} else if resp != nil {
				err = s.InteractionRespond(i.Interaction, resp)
				if err != nil {
					log.Println("error issuing interaction response: ", err)
					iBytes, err := json.Marshal(i.Interaction)
					if err != nil {
						log.Println(err)
					} else {
						log.Println(string(iBytes))
					}
				}
			}
			// no matter what we get back, return
			return
		}
	}
}

func (bot *Bot) slashCommandHandler(s *discordgo.Session, i *discordgo.InteractionCreate) *discordgo.InteractionResponse {
	if i.Member != nil && i.Member.User != nil {
		if redis_common.IsUserBanned(bot.RedisInterface.client, i.Member.User.ID) {
			return nil
		}
	}

	// lock this particular interaction message so no other shard tries to process it
	interactionLock := bot.RedisInterface.LockSnowflake(i.ID)
	// couldn't obtain lock; bail bail bail!
	if interactionLock == nil {
		return nil
	}
	defer server.RecordDiscordRequests(bot.RedisInterface.client, server.MessageCreateDelete, 1)
	defer interactionLock.Release(ctx)

	sett := bot.StorageInterface.GetGuildSettings(i.GuildID)

	// TODO respond properly for commands that *can* be performed in DMs. Such as minimal stats queries, help, info, etc
	// NOTE: difference between i.Member.User (Server/Guild chat) vs i.User (DMs)
	if i.GuildID == "" || i.Member == nil || i.Member.User == nil {
		return command.DmResponse(sett)
	}

	if redis_common.IsUserRateLimitedGeneral(bot.RedisInterface.client, i.Member.User.ID) {
		banned := redis_common.IncrementRateLimitExceed(bot.RedisInterface.client, i.Member.User.ID)
		return softbanResponse(banned, sett)
	}

	g, err := s.State.Guild(i.GuildID)
	if err != nil {
		log.Println(err)
		return command.PrivateErrorResponse("get-guild", err, sett)
	}
	perm, err := bot.PrimarySession.State.UserChannelPermissions(s.State.User.ID, i.ChannelID)
	if err != nil {
		log.Println(err)
		return command.PrivateErrorResponse("get-permissions", err, sett)
	}
	missingPerms := checkPermissions(perm, RequiredPermissions)
	if missingPerms > 0 {
		return command.ReinviteMeResponse(missingPerms, i.ChannelID, sett)
	}

	isAdmin, isPermissioned := false, false
	if g.OwnerID == i.Member.User.ID || (len(sett.AdminUserIDs) == 0 && len(sett.PermissionRoleIDs) == 0) {
		// the guild owner should always have both permissions
		// or if both permissions are still empty, everyone gets both
		isAdmin = true
		isPermissioned = true
	} else {
		// if we have no admins, then we MUST have mods as per the check above. So ensure this user is a mod
		if len(sett.AdminUserIDs) == 0 {
			isAdmin = sett.HasRolePerms(i.Member)
		} else {
			// we have admins; make sure user is one
			isAdmin = sett.HasAdminPerms(i.Member.User)
		}
		// even if we have admins, we can grant mod if the moderators role is empty; it is lesser permissions
		isPermissioned = len(sett.PermissionRoleIDs) == 0 || sett.HasRolePerms(i.Member)
	}

	// common gsr, but not necessarily used by all commands
	gsr := GameStateRequest{
		GuildID:     i.GuildID,
		TextChannel: i.ChannelID,
	}

	if i.Type == discordgo.InteractionApplicationCommand {
		if redis_common.IsUserRateLimitedSpecific(bot.RedisInterface.client, i.Member.User.ID, i.ApplicationCommandData().Name) {
			banned := redis_common.IncrementRateLimitExceed(bot.RedisInterface.client, i.Member.User.ID)
			return softbanResponse(banned, sett)
		}
		var cmdRatelimitTimeout = redis_common.GlobalUserRateLimitDuration
		// /new has a longer ratelimit window than other commands (it's an expensive operation)
		if i.ApplicationCommandData().Name == command.New.Name {
			cmdRatelimitTimeout = redis_common.NewGameRateLimitDuration
		}
		redis_common.MarkUserRateLimit(bot.RedisInterface.client, i.Member.User.ID, i.ApplicationCommandData().Name, cmdRatelimitTimeout)
		switch i.ApplicationCommandData().Name {
		case command.Help.Name:
			return command.HelpResponse(sett, i.ApplicationCommandData().Options)

		case command.Info.Name:
			botInfo := bot.getInfo()
			return command.InfoResponse(botInfo, i.GuildID, sett)

		case command.Link.Name:
			if !isPermissioned {
				return command.InsufficientPermissionsResponse(sett)
			}
			userID, color := command.GetLinkParams(s, i.ApplicationCommandData().Options)

			lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(gsr, 5)
			if lock == nil {
				log.Printf("No lock could be obtained when linking for guild %s, channel %s\n", i.GuildID, i.ChannelID)
				return command.DeadlockGameStateResponse(command.Link.Name, sett)
			}
			if dgs == nil {
				bot.RedisInterface.SetDiscordGameState(nil, lock)
				log.Printf("No game state was available when linking for guild %s, channel %s\n", i.GuildID, i.ChannelID)
				return command.NoGameResponse(sett)
			}
			resp, success := bot.linkOrUnlinkAndRespond(dgs, userID, color, sett)
			if success {
				bot.RedisInterface.SetDiscordGameState(dgs, lock)
				bot.DispatchRefreshOrEdit(dgs, gsr, sett)
			} else {
				// release the lock
				bot.RedisInterface.SetDiscordGameState(nil, lock)
			}
			return resp

		case command.Unlink.Name:
			if !isPermissioned {
				return command.InsufficientPermissionsResponse(sett)
			}
			userID := command.GetUnlinkParams(s, i.ApplicationCommandData().Options)

			lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLock(gsr)
			if lock == nil {
				log.Printf("No lock could be obtained when unlinking for guild %s, channel %s\n", i.GuildID, i.ChannelID)
				return command.DeadlockGameStateResponse(command.Unlink.Name, sett)
			}
			if dgs == nil {
				bot.RedisInterface.SetDiscordGameState(nil, lock)
				log.Printf("No game state was available when unlinking for guild %s, channel %s\n", i.GuildID, i.ChannelID)
				return command.NoGameResponse(sett)
			}
			resp, success := bot.linkOrUnlinkAndRespond(dgs, userID, "", sett)
			if success {
				bot.RedisInterface.SetDiscordGameState(dgs, lock)
				bot.DispatchRefreshOrEdit(dgs, gsr, sett)
			} else {
				// release the lock
				bot.RedisInterface.SetDiscordGameState(nil, lock)
			}
			return resp

		case command.Settings.Name:
			if !isAdmin {
				return command.InsufficientPermissionsResponse(sett)
			}
			premStatus, days, err := bot.PostgresInterface.GetGuildOrUserPremiumStatus(bot.official, bot.TopGGClient, i.GuildID, i.Member.User.ID)
			if err != nil {
				log.Println("Err in /settings get premium:", err)
			}
			setting, args := command.GetSettingsParams(i.ApplicationCommandData().Options)
			msg := bot.HandleSettingsCommand(i.GuildID, sett, setting, args, !premium.IsExpired(premStatus, days))
			return command.SettingsResponse(msg)

		case command.New.Name:
			if !isPermissioned {
				return command.InsufficientPermissionsResponse(sett)
			}

			voiceChannelID := getTrackingChannel(g, i.Member.User.ID)
			if voiceChannelID == "" {
				return command.NewResponse(command.NewNoVoiceChannel, command.NewInfo{}, sett)
			}

			perm, err = bot.PrimarySession.State.UserChannelPermissions(s.State.User.ID, voiceChannelID)
			missingPerms = checkPermissions(perm, VoicePermissions)
			if missingPerms > 0 {
				return command.ReinviteMeResponse(missingPerms, voiceChannelID, sett)
			}

			lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(gsr, 5)
			if lock == nil {
				log.Printf("No lock could be obtained when making a new game for guild %s, channel %s\n", i.GuildID, i.ChannelID)
				return command.DeadlockGameStateResponse(command.New.Name, sett)
			}

			status, activeGames := bot.newGame(dgs)
			if status == command.NewSuccess {
				// release the lock
				bot.RedisInterface.SetDiscordGameState(dgs, lock)

				bot.RedisInterface.RefreshActiveGame(dgs.GuildID, dgs.ConnectCode)

				killChan := make(chan EndGameMessage, 1)
				bot.registerEndGameChannel(dgs.ConnectCode, killChan)
				bot.startGameSubscriber(i.GuildID, dgs.ConnectCode, killChan)

				hyperlink, apiHyperlink, minimalURL := formCaptureURL(bot.url, dgs.ConnectCode)

				bot.handleGameStartMessage(i.GuildID, i.ChannelID, voiceChannelID, i.Member.User.ID, sett, g, dgs.ConnectCode)

				// ===== 修正: 返却レスポンスに /link & /stop ボタンを追加 =====
				resp := command.NewResponse(status, command.NewInfo{
					Hyperlink:    hyperlink,
					ApiHyperlink: apiHyperlink,
					MinimalURL:   minimalURL,
					ConnectCode:  dgs.ConnectCode,
					ActiveGames:  activeGames, // not actually needed for Success messages
				}, sett)

				if resp != nil && resp.Data != nil {
					starterID := i.Member.User.ID
					resp.Data.Components = append(resp.Data.Components, stopButtonComponents(starterID, dgs.ConnectCode, sett)...)

					// 念のためエフェメラル化（スクショの場所に出すため）
					if resp.Data.Flags == 0 {
						resp.Data.Flags = 1 << 6
					}
				}
				return resp

			} else {
				// release the lock
				bot.RedisInterface.SetDiscordGameState(nil, lock)
				return command.NewResponse(status, command.NewInfo{
					ActiveGames: activeGames, // only field we need for success messages
				}, sett)
			}

		case command.Refresh.Name:
			if bot.RefreshGameStateMessage(gsr, sett) {
				return command.PrivateResponse(ThumbsUp)
			} else {
				return command.NoGameResponse(sett)
			}

		case command.Pause.Name:
			if !isPermissioned {
				return command.InsufficientPermissionsResponse(sett)
			}
			lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(gsr, 5)
			if lock == nil {
				log.Printf("No lock could be obtained when pausing game for guild %s, channel %s\n", i.GuildID, i.ChannelID)
				return command.DeadlockGameStateResponse(command.Pause.Name, sett)
			}
			if !dgs.GameStateMsg.Exists() {
				bot.RedisInterface.SetDiscordGameState(nil, lock)
				return command.NoGameResponse(sett)
			}

			dgs.Running = !dgs.Running

			bot.RedisInterface.SetDiscordGameState(dgs, lock)
			// if we paused the game, unmute/undeafen all players
			if !dgs.Running {
				err = bot.applyToAll(dgs, false, false)
			}
			bot.DispatchRefreshOrEdit(dgs, gsr, sett)
			if err != nil {
				return command.PrivateErrorResponse(command.Pause.Name, err, sett)
			}
			return command.PrivateResponse(ThumbsUp)

		case command.End.Name:
			if !isPermissioned {
				return command.InsufficientPermissionsResponse(sett)
			}
			dgs := bot.RedisInterface.GetReadOnlyDiscordGameState(gsr)
			if dgs != nil {
				if !dgs.GameStateMsg.Exists() {
					return command.NoGameResponse(sett)
				}

				// The subscriber performs the normal fail-safe unmute and cleanup.
				// If its channel is missing, fall back to doing the cleanup here.
				if !bot.signalEndGame(dgs.ConnectCode) {
					err = bot.applyToAll(dgs, false, false)
					if err != nil {
						return command.PrivateErrorResponse(command.End.Name, err, sett)
					}
					bot.forceEndGame(gsr)
				}
				return command.PrivateResponse(ThumbsUp)
			}
			return command.DeadlockGameStateResponse(command.End.Name, sett)

		case command.Privacy.Name:
			privArg := command.GetPrivacyParam(i.ApplicationCommandData().Options)
			switch privArg {
			case command.PrivacyInfo:
				return command.PrivacyResponse(privArg, nil, nil, nil, sett)

			case command.PrivacyOptOut:
				err = bot.RedisInterface.DeleteLinksByUserID(i.GuildID, i.Member.User.ID)
				if err != nil {
					return command.PrivacyResponse(privArg, nil, nil, err, sett)
				}
				fallthrough
			case command.PrivacyOptIn:
				err = bot.PostgresInterface.OptUserByString(i.Member.User.ID, privArg == command.PrivacyOptIn)
				return command.PrivacyResponse(privArg, nil, nil, err, sett)

			case command.PrivacyShowMe:
				cached, _ := bot.RedisInterface.GetUsernameOrUserIDMappings(i.GuildID, i.Member.User.ID)
				user, err := bot.PostgresInterface.GetUserByString(i.Member.User.ID)
				return command.PrivacyResponse(privArg, cached, user, err, sett)
			}

		case command.Map.Name:
			mapType, detailed := command.GetMapParams(i.ApplicationCommandData().Options)
			return command.MapResponse(mapType, detailed)

		case command.Stats.Name:
			action, opType, id := command.GetStatsParams(bot.PrimarySession, i.GuildID, i.ApplicationCommandData().Options)
			prem := true
			tier, days, err := bot.PostgresInterface.GetGuildOrUserPremiumStatus(bot.official, bot.TopGGClient, i.GuildID, i.Member.User.ID)
			if err != nil {
				log.Println("Error in /stats getPremium:", err)
			}
			if premium.IsExpired(tier, days) {
				prem = false
			}
			if action == setting.View {
				var embed *discordgo.MessageEmbed
				switch opType {
				case command.User:
					embed = bot.UserStatsEmbed(id, i.GuildID, sett, prem)
				case command.Guild:
					embed = bot.GuildStatsEmbed(i.GuildID, sett, prem)
				case command.Match:
					if MatchIDRegex.Match([]byte(id)) {
						tokens := strings.Split(id, ":")
						embed = bot.GameStatsEmbed(i.GuildID, tokens[1], tokens[0], prem, sett)
					} else {
						err := fmt.Errorf("invalid match code provided: %s, should resemble something like `1A2B3C4D:12345`", id)
						return command.PrivateErrorResponse(command.Stats.Name+" "+command.Match, err, sett)
					}
				}
				if embed != nil {
					return &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Embeds: []*discordgo.MessageEmbed{
								embed,
							},
						},
					}
				}
			} else if action == setting.Clear {
				// id mismatch applies to user ids AND guild ID (guildId *always* != author.id, therefore, must be admin)
				if id != i.Member.User.ID && !isAdmin {
					return command.InsufficientPermissionsResponse(sett)
				}
				var content string
				var components []discordgo.MessageComponent
				switch opType {
				case command.User:
					content = sett.LocalizeMessage(&i18n.Message{
						ID:    "commands.stats.user.reset.confirmation",
						Other: "⚠️**確認**⚠️\n{{.User}} の統計情報をリセットしますか？\nこの操作は取り消せません。",
					},
						map[string]interface{}{
							"User": discord.MentionByUserID(id),
						})
					components = confirmationComponents(resetUserConfirmedID, resetUserCanceledID, sett)
				case command.Guild:
					content = sett.LocalizeMessage(&i18n.Message{
						ID:    "commands.stats.guild.reset.confirmation",
						Other: "⚠️**確認**⚠️\n**{{.Guild}}** の統計情報をリセットしますか？\nこの操作は取り消せません。",
					},
						map[string]interface{}{
							"Guild": g.Name,
						})
					components = confirmationComponents(resetGuildConfirmedID, resetGuildCanceledID, sett)
				}
				return &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Flags:      1 << 6, //private message
						Content:    content,
						Components: components,
					},
				}
			}

		case command.Premium.Name:
			premArg := command.GetPremiumParams(i.ApplicationCommandData().Options)
			premStatus, days, err := bot.PostgresInterface.GetGuildOrUserPremiumStatus(bot.official, bot.TopGGClient, i.GuildID, i.Member.User.ID)
			if err != nil {
				log.Println("Err in /premium get guild prem:", err)
			}
			if premium.IsExpired(premStatus, days) {
				premStatus = premium.FreeTier
			}
			return command.PremiumResponse(i.GuildID, premStatus, days, premArg, isAdmin, sett)

		case command.Debug.Name:
			action, opType, id := command.GetDebugParams(bot.PrimarySession, i.Member.User.ID, i.ApplicationCommandData().Options)
			if action == setting.View {
				if opType == command.User {
					cached, err := bot.RedisInterface.GetUsernameOrUserIDMappings(i.GuildID, id)
					log.Println("View user cache")
					return command.DebugResponse(setting.View, cached, nil, id, err, sett)
				} else if opType == command.GameState {
					state := bot.RedisInterface.GetReadOnlyDiscordGameState(gsr)
					if state != nil {
						jBytes, err := json.MarshalIndent(state, "", "  ")
						return command.DebugResponse(setting.View, nil, jBytes, id, err, sett)
					} else {
						return command.DeadlockGameStateResponse(command.Debug.Name, sett)
					}
				}
			} else if action == setting.Clear {
				if opType == command.User {
					if id != i.Member.User.ID {
						if !isAdmin {
							return command.InsufficientPermissionsResponse(sett)
						}
					}
					err := bot.RedisInterface.DeleteLinksByUserID(i.GuildID, id)
					return command.DebugResponse(setting.Clear, nil, nil, id, err, sett)
				}
			} else if action == command.Unmute {
				// prob shouldn't be constructing the GameState explicitly like this... okay so long as we don't reuse it
				dgs := GameState{
					GuildID:     i.GuildID,
					ConnectCode: i.GuildID + "-unmute",
				}
				// admins can always unmute no matter what
				if isAdmin {
					err = bot.applyToSingle(&dgs, id, false, false)
					if err != nil {
						return command.PrivateErrorResponse(command.Unmute, err, sett)
					}
					return command.PrivateResponse(ThumbsUp)
				}

				for _, v := range g.VoiceStates {
					if v.UserID == id {
						// fetch the game state purely by the voice channel ID
						gsr.TextChannel = ""
						gsr.VoiceChannel = v.ChannelID
						log.Println("fetching game by id ", v.ChannelID)

						// no game is happening in this voice channel, so we're safe to unmute
						if bot.RedisInterface.getDiscordGameStateKey(gsr) == "" {
							err = bot.applyToSingle(&dgs, id, false, false)
							if err != nil {
								return command.PrivateErrorResponse(command.Unmute, err, sett)
							}
							return command.PrivateResponse(ThumbsUp)
						} else {
							// there's a game happening, so we can't unmute
							return command.DebugResponse(command.Unmute, nil, nil, id, errors.New(""), sett)
						}
					}
				}
				return command.PrivateErrorResponse(command.Unmute, errors.New("user is not in a voice channel"), sett)
			} else if action == command.UnmuteAll {
				dgs := bot.RedisInterface.GetReadOnlyDiscordGameState(gsr)
				if dgs != nil {
					err = bot.applyToAll(dgs, false, false)
					if err != nil {
						return command.PrivateErrorResponse(command.UnmuteAll, err, sett)
					}
					return command.PrivateResponse(ThumbsUp)
				}
				return command.DeadlockGameStateResponse(command.UnmuteAll, sett)
			}

		case command.Download.Name:
			if !isAdmin {
				return command.InsufficientPermissionsResponse(sett)
			}
			// don't send the userid because downloading is restricted to Gold members
			premStatus, days, err := bot.PostgresInterface.GetGuildOrUserPremiumStatus(bot.official, bot.TopGGClient, i.GuildID, "")
			if err != nil {
				log.Println("Err in /download get guild prem:", err)
			}
			if premium.IsExpired(premStatus, days) {
				premStatus = premium.FreeTier
			}
			if premStatus != premium.SelfHostTier && premStatus != premium.GoldTier {
				return command.DownloadNotGoldResponse(sett)
			}
			missingPerms = checkPermissions(perm, DownloadPermissions)
			if missingPerms > 0 {
				return command.ReinviteMeResponse(missingPerms, i.ChannelID, sett)
			}

			category := command.GetDownloadParams(i.ApplicationCommandData().Options)

			d, err := redis_common.GetDownloadCategoryCooldown(bot.RedisInterface.client, i.GuildID, category)
			if err != nil {
				return command.PrivateErrorResponse("/download guild", err, sett)
			}
			if d > 0 {
				return command.DownloadCooldownResponse(sett, category, d)
			}
			var content string
			var components []discordgo.MessageComponent
			content = sett.LocalizeMessage(&i18n.Message{
				ID:    "commands.download.guild.confirmation",
				Other: "⚠️**確認**⚠️\n`{{.Category}}` のデータをダウンロードすると、24時間は再ダウンロードできません。",
			}, map[string]interface{}{
				"Category": category,
			})
			var downloadConfirmedID string
			switch category {
			case command.Guild:
				downloadConfirmedID = downloadGuildConfirmedID
			case command.Users:
				downloadConfirmedID = downloadUsersConfirmedID
			case command.UsersGames:
				downloadConfirmedID = downloadUsersGamesConfirmedID
			case command.Games:
				downloadConfirmedID = downloadGamesConfirmedID
			case command.GameEvents:
				downloadConfirmedID = downloadGameEventsConfirmedID
			}
			components = confirmationComponents(downloadConfirmedID, downloadCanceledID, sett)
			return &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Flags:      1 << 6, //private message
					Content:    content,
					Components: components,
				},
			}
		}

	} else if i.Type == discordgo.InteractionMessageComponent {
		if redis_common.IsUserRateLimitedSpecific(bot.RedisInterface.client, i.Member.User.ID, i.MessageComponentData().CustomID) {
			banned := redis_common.IncrementRateLimitExceed(bot.RedisInterface.client, i.Member.User.ID)
			return softbanResponse(banned, sett)
		}
		redis_common.MarkUserRateLimit(bot.RedisInterface.client, i.Member.User.ID, i.MessageComponentData().CustomID, redis_common.GlobalUserRateLimitDuration)

		gid, err := strconv.ParseUint(i.GuildID, 10, 64)
		if err != nil {
			log.Println(err)
			// TODO report this properly
		}

		customID := i.MessageComponentData().CustomID

		switch {
		// ========= 追加: /link ボタン（起動者 → 対象ユーザー選択） =========
		case strings.HasPrefix(customID, linkButtonIDPrefix):
			// CustomID: "link-game:<starterUserID>:<connectCode>"
			parts := strings.SplitN(customID, ":", 3)
			if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
				return command.PrivateResponse(staleStartControlMessage)
			}
			starterID := parts[1]
			connectCode := parts[2]

			// 起動者以外は拒否（エフェメラルだが念のため）
			if starterID != "" && i.Member != nil && i.Member.User != nil && i.Member.User.ID != starterID {
				msg := sett.LocalizeMessage(&i18n.Message{
					ID:    "commands.link.onlyStarter",
					Other: "このボタンは /start（ゲーム開始）を実行した起動者のみ押せます。",
				})
				return command.PrivateResponse(msg)
			}

			currentGame := bot.RedisInterface.GetReadOnlyDiscordGameState(gsr)
			if !isCurrentStartControlGame(currentGame, connectCode) {
				return command.PrivateResponse(staleStartControlMessage)
			}

			// 起動者のいるボイスチャンネル取得
			g, err := bot.PrimarySession.State.Guild(i.GuildID)
			if err != nil {
				log.Println(err)
				return command.PrivateErrorResponse("get-guild", err, sett)
			}
			voiceChannelID := getTrackingChannel(g, starterID)
			if voiceChannelID == "" {
				msg := sett.LocalizeMessage(&i18n.Message{
					ID:    "commands.link.noVoice",
					Other: "起動者がボイスチャンネルにいません。",
				})
				return command.PrivateResponse(msg)
			}

			// ボイスチャンネル内のメンバー一覧を SelectMenu にする
			options := []discordgo.SelectMenuOption{}
			for _, vs := range g.VoiceStates {
				if vs.ChannelID == voiceChannelID {
					// vs.UserID からメンバー名取得。キャッシュに無い場合だけAPIへ問い合わせます。
					member, err := bot.PrimarySession.State.Member(g.ID, vs.UserID)
					if err != nil || member == nil || member.User == nil {
						member, err = bot.PrimarySession.GuildMember(g.ID, vs.UserID)
					}
					if err != nil || member == nil || member.User == nil {
						continue
					}
					label := resolveDiscordDisplayName(
						bot.PrimarySession,
						g.ID,
						member.User.ID,
						member.Nick,
						member.User.Username,
					)
					if len([]rune(label)) > 100 {
						label = string([]rune(label)[:100])
					}

					options = append(options, discordgo.SelectMenuOption{
						Label: label,
						Value: member.User.ID,
						Emoji: discordgo.ComponentEmoji{
							Name: "👤", // ★ここを追加：適当な Unicode 絵文字なら何でもOK
						},
					})
				}
			}

			if len(options) == 0 {
				msg := sett.LocalizeMessage(&i18n.Message{
					ID:    "commands.link.noMembers",
					Other: "ボイスチャンネルにメンバーがいません。",
				})
				return command.PrivateResponse(msg)
			}

			selectCustomID := fmt.Sprintf("%s:%s:%s", linkUserSelectPrefix, starterID, connectCode)

			// ★★★ 修正ポイント: MinValues は *int なので変数を用意して & を渡す ★★★
			min := 1

			components := []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.SelectMenu{
							CustomID:    selectCustomID,
							Options:     options,
							Placeholder: "リンクするプレイヤーを選択してください",
							MinValues:   &min,
							MaxValues:   1,
						},
					},
				},
			}

			return &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Flags:      1 << 6, // エフェメラル
					Content:    "リンクしたいプレイヤーを選択してください。",
					Components: components,
				},
			}

		// ========= 追加: /link ユーザー選択後 → 色ボタン表示 =========
		case strings.HasPrefix(customID, linkUserSelectPrefix):
			// CustomID: "link-select-user:<starterUserID>:<connectCode>"
			parts := strings.SplitN(customID, ":", 3)
			if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
				return command.PrivateResponse(staleStartControlMessage)
			}
			starterID := parts[1]
			connectCode := parts[2]

			if starterID != "" && i.Member != nil && i.Member.User != nil && i.Member.User.ID != starterID {
				msg := sett.LocalizeMessage(&i18n.Message{
					ID:    "commands.link.onlyStarter",
					Other: "この操作は /start（ゲーム開始）を実行した起動者のみ行えます。",
				})
				return command.PrivateResponse(msg)
			}

			currentGame := bot.RedisInterface.GetReadOnlyDiscordGameState(gsr)
			if !isCurrentStartControlGame(currentGame, connectCode) {
				return command.PrivateResponse(staleStartControlMessage)
			}

			values := i.MessageComponentData().Values
			if len(values) == 0 {
				msg := sett.LocalizeMessage(&i18n.Message{
					ID:    "commands.link.noUserSelected",
					Other: "プレイヤーが選択されていません。",
				})
				return command.PrivateResponse(msg)
			}

			targetUserID := values[0]

			// 色ボタンを表示
			components := linkColorButtons(starterID, targetUserID, connectCode)

			content := fmt.Sprintf("%s に割り当てる色を選択してください。", discord.MentionByUserID(targetUserID))

			return &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
				Data: &discordgo.InteractionResponseData{
					Flags:      1 << 6, // エフェメラルのまま
					Content:    content,
					Components: components,
				},
			}

		// ========= 追加: /link 色ボタン → 実際にリンク処理 =========
		case strings.HasPrefix(customID, linkColorButtonPrefix):
			// CustomID: "link-color:<starterUserID>:<targetUserID>:<connectCode>:<ColorName>"
			parts := strings.SplitN(customID, ":", 5)
			if len(parts) != 5 || parts[1] == "" || parts[2] == "" || parts[3] == "" {
				msg := sett.LocalizeMessage(&i18n.Message{
					ID:    "commands.link.invalidColor",
					Other: "リンク用のボタン情報が不正です。",
				})
				return command.PrivateResponse(msg)
			}
			starterID := parts[1]
			targetUserID := parts[2]
			connectCode := parts[3]
			colorName := parts[4]

			if starterID != "" && i.Member != nil && i.Member.User != nil && i.Member.User.ID != starterID {
				msg := sett.LocalizeMessage(&i18n.Message{
					ID:    "commands.link.onlyStarter",
					Other: "この操作は /start（ゲーム開始）を実行した起動者のみ行えます。",
				})
				return command.PrivateResponse(msg)
			}

			// ゲーム状態取得
			gsr := GameStateRequest{
				GuildID:     i.GuildID,
				TextChannel: i.ChannelID,
			}

			lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(gsr, 5)
			if lock == nil {
				log.Printf("No lock could be obtained when linking for guild %s, channel %s\n", i.GuildID, i.ChannelID)
				return command.DeadlockGameStateResponse(command.Link.Name, sett)
			}
			if dgs == nil {
				bot.RedisInterface.SetDiscordGameState(nil, lock)
				return command.NoGameResponse(sett)
			}
			if !isCurrentStartControlGame(dgs, connectCode) {
				bot.RedisInterface.SetDiscordGameState(nil, lock)
				return command.PrivateResponse(staleStartControlMessage)
			}

			// UNLINK なら色空文字、それ以外は指定色
			value := colorName
			if colorName == "UNLINK" {
				value = ""
			}

			resp, success := bot.linkOrUnlinkAndRespond(dgs, targetUserID, value, sett)
			if success {
				bot.RedisInterface.SetDiscordGameState(dgs, lock)
				bot.DispatchRefreshOrEdit(dgs, gsr, sett)
			} else {
				// only release the lock; no changes
				bot.RedisInterface.SetDiscordGameState(nil, lock)
			}
			return resp

		// ========= 追加: 表示更新ボタン =========
		case strings.HasPrefix(customID, refreshButtonIDPrefix):
			// CustomID: "refresh-game:<starterUserID>:<connectCode>"
			parts := strings.SplitN(customID, ":", 3)
			if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
				return command.PrivateResponse(staleStartControlMessage)
			}
			starterID := parts[1]
			connectCode := parts[2]

			// 誤操作や連打を避けるため、/start の起動者だけが更新できます。
			if starterID != "" && i.Member != nil && i.Member.User != nil && i.Member.User.ID != starterID {
				return command.PrivateResponse("このボタンは /start（ゲーム開始）を実行した起動者のみ押せます。")
			}

			gsr := GameStateRequest{
				GuildID:     i.GuildID,
				TextChannel: i.ChannelID,
			}
			currentGame := bot.RedisInterface.GetReadOnlyDiscordGameState(gsr)
			if !isCurrentStartControlGame(currentGame, connectCode) {
				return command.PrivateResponse(staleStartControlMessage)
			}
			if !currentGame.GameStateMsg.Exists() {
				return command.NoGameResponse(sett)
			}

			if bot.RefreshGameStateMessage(gsr, sett) {
				return command.PrivateResponse("✅ 表示を更新しました。")
			}
			return command.PrivateResponse("表示を更新できませんでした。少し待ってからもう一度お試しください。")

		// ========= 既存: /stop ボタン =========
		case strings.HasPrefix(customID, stopButtonIDPrefix):
			// CustomID: "stop-game:<starterUserID>:<connectCode>"
			parts := strings.SplitN(customID, ":", 3)
			if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
				return command.PrivateResponse(staleStartControlMessage)
			}
			starterID := parts[1]
			connectCode := parts[2]

			// 起動者以外は拒否
			if starterID != "" && i.Member != nil && i.Member.User != nil && i.Member.User.ID != starterID {
				msg := sett.LocalizeMessage(&i18n.Message{
					ID:    "commands.stop.onlyStarter",
					Other: "このボタンは /start（ゲーム開始）を実行した起動者のみ押せます。",
				})
				return command.PrivateResponse(msg)
			}

			// /end と同じ終了処理
			gsr := GameStateRequest{
				GuildID:     i.GuildID,
				TextChannel: i.ChannelID,
			}
			dgs := bot.RedisInterface.GetReadOnlyDiscordGameState(gsr)
			if dgs != nil {
				if !isCurrentStartControlGame(dgs, connectCode) {
					return command.PrivateResponse(staleStartControlMessage)
				}
				if !dgs.GameStateMsg.Exists() {
					return command.NoGameResponse(sett)
				}

				// The subscriber performs the normal fail-safe unmute and cleanup.
				// If its channel is missing, fall back to doing the cleanup here.
				if !bot.signalEndGame(dgs.ConnectCode) {
					err = bot.applyToAll(dgs, false, false)
					if err != nil {
						return command.PrivateErrorResponse(command.End.Name, err, sett)
					}
					bot.forceEndGame(gsr)
				}
				return command.PrivateResponse(ThumbsUp)
			}
			return command.PrivateResponse(staleStartControlMessage)

		// ========= 色ボタン（元からある自分用 select-color） =========
		case strings.HasPrefix(customID, colorSelectID):
			// CustomID: "select-color:Red" 形式
			value := ""
			parts := strings.SplitN(customID, ":", 2)
			if len(parts) == 2 {
				value = parts[1]
			}

			gsr := GameStateRequest{
				GuildID:     i.GuildID,
				TextChannel: i.ChannelID,
			}

			lock, dgs := bot.RedisInterface.GetDiscordGameStateAndLockRetries(gsr, 5)
			if lock == nil {
				log.Printf("No lock could be obtained when linking for guild %s, channel %s\n", i.GuildID, i.ChannelID)
				return command.DeadlockGameStateResponse(command.Link.Name, sett)
			}
			if dgs == nil {
				bot.RedisInterface.SetDiscordGameState(nil, lock)
				return command.NoGameResponse(sett)
			}

			messageID := ""
			if i.Message != nil {
				messageID = i.Message.ID
			}
			if !isCurrentSelfLinkPanel(dgs, messageID, i.ChannelID) {
				bot.RedisInterface.SetDiscordGameState(nil, lock)
				return command.PrivateResponse("この色選択パネルは古いため使用できません。現在の参加者一覧に表示されているボタンを押してください。")
			}

			if !dgs.CaptureConnected {
				bot.RedisInterface.SetDiscordGameState(nil, lock)
				return command.PrivateResponse("AmongUsCaptureが未接続です。AmongUsCaptureを接続してから色を選択してください。")
			}

			userID := i.Member.User.ID
			if !isUserInTrackedVoiceChannel(g, userID, dgs.VoiceChannel) {
				bot.RedisInterface.SetDiscordGameState(nil, lock)
				return command.PrivateResponse("この色ボタンは、/start を実行した時のボイスチャンネルに参加している人だけ使用できます。対象のボイスチャンネルへ参加してから、もう一度押してください。")
			}

			// VoiceState到着直後などでUserDataがまだ無い場合だけ、安全に追加します。
			if _, ok := dgs.UserData[userID]; !ok {
				if _, added := dgs.checkCacheAndAddUser(g, bot.PrimarySession, userID); !added {
					bot.RedisInterface.SetDiscordGameState(nil, lock)
					return command.PrivateResponse("Discord参加者情報を取得できませんでした。数秒待ってから、もう一度押してください。")
				}
			}

			if value == UnlinkEmojiName {
				value = ""
			} else {
				// 公開色ボタンは未リンク参加者の自己リンク専用です。
				if playerName, linked := linkedPlayerNameForUser(dgs, userID); linked {
					bot.RedisInterface.SetDiscordGameState(nil, lock)
					return command.PrivateResponse(fmt.Sprintf("すでに「%s」へリンクされています。修正が必要な場合はホストに /link 操作を依頼してください。", playerName))
				}

				// 同じゲーム内プレイヤー（色）を別のDiscord参加者が使用中なら奪わないようにします。
				if selectedPlayer, found := dgs.GameData.GetByColor(value); found {
					if linkedUserID, used := linkedUserForPlayerName(dgs, selectedPlayer.Name, userID); used {
						bot.RedisInterface.SetDiscordGameState(nil, lock)
						return command.PrivateResponse(fmt.Sprintf("この色はすでに %s がリンクしています。別の色を選ぶか、ホストに /link 操作を依頼してください。", discord.MentionByUserID(linkedUserID)))
					}
				}
			}

			resp, success := bot.linkOrUnlinkAndRespond(dgs, userID, value, sett)
			if success {
				bot.RedisInterface.SetDiscordGameState(dgs, lock)
				bot.DispatchRefreshOrEdit(dgs, gsr, sett)
			} else {
				// only release the lock; no changes
				bot.RedisInterface.SetDiscordGameState(nil, lock)
			}
			return resp

		// ========= 以下、元々の reset/download ボタン =========
		case customID == resetUserConfirmedID:
			var content string
			if len(i.Message.Mentions) == 1 {
				id := i.Message.Mentions[0].ID
				err := bot.PostgresInterface.DeleteAllGamesForUser(id)
				if err != nil {
					content = sett.LocalizeMessage(&i18n.Message{
						ID:    "commands.stats.user.reset.error",
						Other: "{{.User}} の統計情報をリセット中にエラーが発生しました：{{.Error}}",
					},
						map[string]interface{}{
							"User":  discord.MentionByUserID(id),
							"Error": err.Error(),
						})
				} else {
					content = sett.LocalizeMessage(&i18n.Message{
						ID:    "commands.stats.user.reset.success",
						Other: "{{.User}} の統計情報をリセットしました。",
					},
						map[string]interface{}{
							"User": discord.MentionByUserID(id),
						})
				}
			} else {
				content = sett.LocalizeMessage(&i18n.Message{
					ID:    "commands.stats.user.reset.notfound",
					Other: "メッセージからユーザー情報を取得できませんでした。",
				})
			}
			if i.Message.MessageReference != nil {
				bot.deleteComponentInParentMessage(s, i)
			}
			return &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
				Data: &discordgo.InteractionResponseData{
					Flags:      1 << 6, //private message
					Content:    content,
					Components: []discordgo.MessageComponent{},
				},
			}

		case customID == resetGuildConfirmedID:
			var content string
			err := bot.PostgresInterface.DeleteAllGamesForServer(i.GuildID)
			if err != nil {
				content = sett.LocalizeMessage(&i18n.Message{
					ID:    "commands.stats.guild.reset.error",
					Other: "このサーバーの統計情報をリセット中にエラーが発生しました：{{.Error}}",
				},
					map[string]interface{}{
						"Error": err.Error(),
					})
			} else {
				content = sett.LocalizeMessage(&i18n.Message{
					ID:    "commands.stats.guild.reset.success",
					Other: "**{{.Guild}}** の統計情報をリセットしました。",
				},
					map[string]interface{}{
						"Guild": g.Name,
					})
			}
			if i.Message.MessageReference != nil {
				bot.deleteComponentInParentMessage(s, i)
			}
			return &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
				Data: &discordgo.InteractionResponseData{
					Flags:      1 << 6, //private message
					Content:    content,
					Components: []discordgo.MessageComponent{},
				},
			}

		case customID == downloadGuildConfirmedID:
			guild, err := bot.PostgresInterface.GetGuildForDownload(gid)
			if err != nil {
				log.Println("Error downloading guild data:", err)
				return downloadErrorResponse(sett, err)
			} else {
				redis_common.MarkDownloadCategoryCooldown(bot.RedisInterface.client, i.GuildID, command.Guild)
				return &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseUpdateMessage,
					Data: &discordgo.InteractionResponseData{
						Flags: 1 << 6, //private message
						Content: sett.LocalizeMessage(&i18n.Message{
							ID:    "commands.download.file.success",
							Other: "ファイルを作成しました。",
						}),
						Components: []discordgo.MessageComponent{},
						Files: []*discordgo.File{
							{
								Name:        "guilds.csv",
								ContentType: "text/csv",
								Reader:      strings.NewReader(guild.ToCSV()),
							},
						},
					},
				}
			}

		case customID == downloadUsersConfirmedID:
			users, err := bot.PostgresInterface.GetUsersForGuild(gid)
			if err != nil {
				log.Println("Error downloading users data:", err)
				return downloadErrorResponse(sett, err)
			} else {
				redis_common.MarkDownloadCategoryCooldown(bot.RedisInterface.client, i.GuildID, command.Users)
				return &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseUpdateMessage,
					Data: &discordgo.InteractionResponseData{
						Flags: 1 << 6, //private message
						Content: sett.LocalizeMessage(&i18n.Message{
							ID:    "commands.download.file.success",
							Other: "ファイルを作成しました。",
						}),
						Components: []discordgo.MessageComponent{},
						Files: []*discordgo.File{
							{
								Name:        "users.csv",
								ContentType: "text/csv",
								Reader:      strings.NewReader(storage.UsersToCSV(users)),
							},
						},
					},
				}
			}

		case customID == downloadUsersGamesConfirmedID:
			usersGames, err := bot.PostgresInterface.GetUsersGamesForGuild(gid)
			if err != nil {
				log.Println("Error downloading users_games data:", err)
				return downloadErrorResponse(sett, err)
			} else {
				redis_common.MarkDownloadCategoryCooldown(bot.RedisInterface.client, i.GuildID, command.UsersGames)
				return &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseUpdateMessage,
					Data: &discordgo.InteractionResponseData{
						Flags: 1 << 6, //private message
						Content: sett.LocalizeMessage(&i18n.Message{
							ID:    "commands.download.file.success",
							Other: "ファイルを作成しました。",
						}),
						Components: []discordgo.MessageComponent{},
						Files: []*discordgo.File{
							{
								Name:        "users_games.csv",
								ContentType: "text/csv",
								Reader:      strings.NewReader(storage.UsersGamesToCSV(usersGames)),
							},
						},
					},
				}
			}

		case customID == downloadGamesConfirmedID:
			games, err := bot.PostgresInterface.GetGamesForGuild(gid)
			if err != nil {
				log.Println("Error downloading game data:", err)
				return downloadErrorResponse(sett, err)
			} else {
				redis_common.MarkDownloadCategoryCooldown(bot.RedisInterface.client, i.GuildID, command.Games)
				return &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseUpdateMessage,
					Data: &discordgo.InteractionResponseData{
						Flags: 1 << 6, //private message
						Content: sett.LocalizeMessage(&i18n.Message{
							ID:    "commands.download.file.success",
							Other: "ファイルを作成しました。",
						}),
						Components: []discordgo.MessageComponent{},
						Files: []*discordgo.File{
							{
								Name:        "games.csv",
								ContentType: "text/csv",
								Reader:      strings.NewReader(storage.GamesToCSV(games)),
							},
						},
					},
				}
			}

		case customID == downloadGameEventsConfirmedID:
			events, err := bot.PostgresInterface.GetGamesEventsForGuild(gid)
			if err != nil {
				log.Println("Error downloading game events data:", err)
				return downloadErrorResponse(sett, err)
			} else {
				redis_common.MarkDownloadCategoryCooldown(bot.RedisInterface.client, i.GuildID, command.GameEvents)
				return &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseUpdateMessage,
					Data: &discordgo.InteractionResponseData{
						Flags: 1 << 6, //private message
						Content: sett.LocalizeMessage(&i18n.Message{
							ID:    "commands.download.file.success",
							Other: "ファイルを作成しました。",
						}),
						Components: []discordgo.MessageComponent{},
						Files: []*discordgo.File{
							{
								Name:        "events.csv",
								ContentType: "text/csv",
								Reader:      strings.NewReader(storage.EventsToCSV(events)),
							},
						},
					},
				}
			}

		case customID == downloadCanceledID,
			customID == resetUserCanceledID,
			customID == resetGuildCanceledID:
			if i.Message.MessageReference != nil {
				bot.deleteComponentInParentMessage(s, i)
			}
			return resetCancelResponse(sett)
		}
	}

	// no command or handler matched somehow
	return nil
}

func downloadErrorResponse(sett *settings.GuildSettings, err error) *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Flags: 1 << 6, //private message
			Content: sett.LocalizeMessage(&i18n.Message{
				ID:    "commands.download.guild.error",
				Other: "データの取得中にエラーが発生しました：{{.Error}}",
			}, map[string]interface{}{
				"Error": err.Error(),
			}),
		},
	}
}

func (bot *Bot) linkOrUnlinkAndRespond(dgs *GameState, userID, testValue string, sett *settings.GuildSettings) (*discordgo.InteractionResponse, bool) {
	if dgs == nil {
		return command.NoGameResponse(sett), false
	}
	if testValue != "" {
		// don't care if it's successful, just always unlink before linking
		unlinkPlayer(dgs, userID)
		status, err := linkPlayer(bot.RedisInterface, dgs, userID, testValue)
		if err != nil {
			log.Println(err)
		}
		return command.LinkResponse(status, userID, testValue, sett), status == command.LinkSuccess
	} else {
		status := unlinkPlayer(dgs, userID)
		return command.UnlinkResponse(status, userID, sett), status == command.UnlinkSuccess
	}
}

// deleteComponentInParentMessage deletes any components from parent messages.
// this is required for safety. if the resetting process takes over 2 seconds,
// since RESET/Cancel buttons remain forever once the button has been clicked.
func (bot *Bot) deleteComponentInParentMessage(s *discordgo.Session, i *discordgo.InteractionCreate) {
	me := discordgo.NewMessageEdit(i.ChannelID, i.Message.ID)
	me.Components = []discordgo.MessageComponent{}
	_, err := s.ChannelMessageEditComplex(me)
	if err != nil {
		log.Println("Error when attempting to edit complex message", err)
	}
}

func confirmationComponents(confirmedID string, canceledID string, sett *settings.GuildSettings) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					CustomID: confirmedID,
					Style:    discordgo.DangerButton,
					Label: sett.LocalizeMessage(&i18n.Message{
						ID:    "commands.stats.reset.button.proceed",
						Other: "実行する",
					}),
					Emoji: discordgo.ComponentEmoji{Name: ThumbsUp},
				},
				discordgo.Button{
					CustomID: canceledID,
					Style:    discordgo.SecondaryButton,
					Label: sett.LocalizeMessage(&i18n.Message{
						ID:    "commands.stats.reset.button.cancel",
						Other: "キャンセル",
					}),
					Emoji: discordgo.ComponentEmoji{Name: X},
				},
			},
		},
	}
}

func resetCancelResponse(sett *settings.GuildSettings) *discordgo.InteractionResponse {
	content := sett.LocalizeMessage(&i18n.Message{
		ID:    "commands.stats.reset.canceled",
		Other: "操作をキャンセルしました。",
	})
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Flags:      1 << 6, //private message
			Content:    content,
			Components: []discordgo.MessageComponent{},
		},
	}
}

func softbanResponse(banned bool, sett *settings.GuildSettings) *discordgo.InteractionResponse {
	if banned {
		return &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags: 1 << 6, //private message
				Content: sett.LocalizeMessage(&i18n.Message{
					ID:    "softban.ignoring",
					Other: "コマンドの連続実行が多いため、5分間操作を受け付けません。",
				}),
			},
		}
	}
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: 1 << 6, //private message
			Content: sett.LocalizeMessage(&i18n.Message{
				ID:    "softban.warning",
				Other: "コマンドを短時間に連続実行しないでください。",
			}),
		},
	}
}

func checkPermissions(perm int64, perms []int64) (a int64) {
	for _, v := range perms {
		if v&perm != v {
			a |= v
		}
	}
	return
}
