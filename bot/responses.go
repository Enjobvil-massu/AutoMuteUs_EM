package bot

import (
	"fmt"
	"time"

	"github.com/automuteus/automuteus/v8/bot/setting"
	"github.com/automuteus/automuteus/v8/pkg/amongus"
	"github.com/automuteus/automuteus/v8/pkg/discord"
	"github.com/automuteus/automuteus/v8/pkg/game"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

const ISO8601 = "2006-01-02T15:04:05-0700"

func settingResponse(settingsList []setting.Setting, sett *settings.GuildSettings, prem bool) *discordgo.MessageEmbed {
	embed := discordgo.MessageEmbed{
		URL:  "",
		Type: "",
		Title: sett.LocalizeMessage(&i18n.Message{
			ID:    "responses.settingResponse.Title",
			Other: "設定",
		}),
		Description: sett.LocalizeMessage(&i18n.Message{
			ID:    "responses.settingResponse.Description",
			Other: "以下の設定は `/settings <設定名>` から変更できます。",
		}),
		Timestamp: "",
		Color:     15844367, // GOLD
		Image:     nil,
		Thumbnail: nil,
		Video:     nil,
		Provider:  nil,
		Author:    nil,
	}

	fields := make([]*discordgo.MessageEmbedField, 0)
	for _, v := range settingsList {
		if !v.Premium {
			name := setting.DisplayName(v.Name)
			fields = append(fields, &discordgo.MessageEmbedField{
				Name:   name,
				Value:  sett.LocalizeMessage(&i18n.Message{Other: v.ShortDesc}),
				Inline: true,
			})
		}
	}
	var desc string
	if prem {
		desc = sett.LocalizeMessage(&i18n.Message{
			ID:    "responses.settingResponse.PremiumThanks",
			Other: "AutoMuteUsプレミアムをご利用いただきありがとうございます。",
		})
	} else {
		desc = sett.LocalizeMessage(&i18n.Message{
			ID:    "responses.settingResponse.PremiumNoThanks",
			Other: "以下はAutoMuteUsプレミアム専用設定です。",
		})
	}
	fields = append(fields, &discordgo.MessageEmbedField{
		Name:   "\u200B",
		Value:  "\u200B",
		Inline: false,
	})
	fields = append(fields, &discordgo.MessageEmbedField{
		Name:   "💎 プレミアム設定 💎",
		Value:  desc,
		Inline: false,
	})
	for _, v := range settingsList {
		if v.Premium {
			name := setting.DisplayName(v.Name)
			fields = append(fields, &discordgo.MessageEmbedField{
				Name:   name,
				Value:  sett.LocalizeMessage(&i18n.Message{Other: v.ShortDesc}),
				Inline: true,
			})
		}
	}

	embed.Fields = fields
	return &embed
}

func (bot *Bot) gameStateResponse(dgs *GameState, sett *settings.GuildSettings) *discordgo.MessageEmbed {
	// ゲームのフェーズに応じて表示を切り替え
	messages := map[game.Phase]func(dgs *GameState, emojis AlivenessEmojis, sett *settings.GuildSettings) *discordgo.MessageEmbed{
		game.MENU:     menuMessage,
		game.LOBBY:    lobbyMessage,
		game.TASKS:    gamePlayMessage,
		game.DISCUSS:  gamePlayMessage,
		game.GAMEOVER: gamePlayMessage,
	}
	return messages[dgs.GameData.Phase](dgs, bot.StatusEmojis, sett)
}

// ===== メタ情報 (ホスト / VC / リンク済人数) =====

func lobbyMetaEmbedFields(room, region string, author, voiceChannelID string, playerCount int, linkedPlayers int, sett *settings.GuildSettings) []*discordgo.MessageEmbedField {
	gameInfoFields := make([]*discordgo.MessageEmbedField, 0)

	// ホスト
	if author != "" {
		gameInfoFields = append(gameInfoFields, &discordgo.MessageEmbedField{
			Name:   "ホスト",
			Value:  discord.MentionByUserID(author),
			Inline: true,
		})
	}

	// ボイスチャンネル
	if voiceChannelID != "" {
		gameInfoFields = append(gameInfoFields, &discordgo.MessageEmbedField{
			Name:   "ボイスチャンネル",
			Value:  discord.MentionByChannelID(voiceChannelID),
			Inline: true,
		})
	}

	// 改行用ダミー
	if len(gameInfoFields) > 0 {
		gameInfoFields = append(gameInfoFields, &discordgo.MessageEmbedField{
			Name:   "\u200B",
			Value:  "\u200B",
			Inline: true,
		})
	}

	// リンク済メンバー（人数が検出数を超えないように補正）
	if linkedPlayers > playerCount {
		linkedPlayers = playerCount
	}
	gameInfoFields = append(gameInfoFields, &discordgo.MessageEmbedField{
		Name:   "リンク済人数／参加人数",
		Value:  fmt.Sprintf("%v/%v", linkedPlayers, playerCount),
		Inline: false,
	})

	// ROOM CODE / REGION は表示しない仕様に変更

	return gameInfoFields
}

// ===== メニュー（待機中） =====

func menuMessage(dgs *GameState, _ AlivenessEmojis, sett *settings.GuildSettings) *discordgo.MessageEmbed {
	var footer *discordgo.MessageEmbedFooter
	desc, color := dgs.descriptionAndColor(sett)
	if color == discord.DEFAULT {
		color = discord.GREEN
		footer = &discordgo.MessageEmbedFooter{
			Text: "(アモアスのロビーに入室すると試合が開始されます)",
		}
	}

	fields := make([]*discordgo.MessageEmbedField, 0)
	author := dgs.GameStateMsg.LeaderID
	if author != "" {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "ホスト",
			Value:  discord.MentionByUserID(author),
			Inline: true,
		})
	}
	if dgs.VoiceChannel != "" {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "ボイスチャンネル",
			Value:  "<#" + dgs.VoiceChannel + ">",
			Inline: true,
		})
	}
	if len(fields) == 2 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "\u200B",
			Value:  "\u200B",
			Inline: true,
		})
	}

	msg := discordgo.MessageEmbed{
		URL:         "",
		Type:        "",
		Title:       "メニュー",
		Description: desc,
		Timestamp:   time.Now().Format(ISO8601),
		Footer:      footer,
		Color:       color,
		Image:       nil,
		Thumbnail:   nil,
		Video:       nil,
		Provider:    nil,
		Author:      nil,
		Fields:      fields,
	}
	return &msg
}

// ===== ロビー中 =====

func lobbyMessage(dgs *GameState, emojis AlivenessEmojis, sett *settings.GuildSettings) *discordgo.MessageEmbed {
	room, region, _ := dgs.GameData.GetRoomRegionMap()
	gameInfoFields := lobbyMetaEmbedFields(room, region, dgs.GameStateMsg.LeaderID, dgs.VoiceChannel, dgs.GameData.GetNumDetectedPlayers(), dgs.GetCountLinked(), sett)

	listResp := dgs.ToEmojiEmbedFields(emojis, sett)
	listResp = append(gameInfoFields, listResp...)

	desc, color := dgs.descriptionAndColor(sett)
	if color == discord.DEFAULT {
		color = discord.GREEN
	}

	msg := discordgo.MessageEmbed{
		URL:         "",
		Type:        "",
		Title:       "ロビー",
		Description: desc,
		Timestamp:   time.Now().Format(ISO8601),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "自動リンクされていない方は、下のボタンから自分の色を選択してください（リンク解除ボタンで解除できます）",
		},
		Color:     color,
		Image:     nil,
		Thumbnail: nil, // 地図サムネは非表示
		Video:     nil,
		Provider:  nil,
		Author:    nil,
		Fields:    listResp,
	}
	return &msg
}

// ===== 試合終了メッセージ =====

func gameOverMessage(dgs *GameState, emojis AlivenessEmojis, sett *settings.GuildSettings, winners string) *discordgo.MessageEmbed {
	_, _, playMap := dgs.GameData.GetRoomRegionMap()

	listResp := dgs.ToEmojiEmbedFields(emojis, sett)

	desc := sett.LocalizeMessage(&i18n.Message{
		ID:    "eventHandler.gameOver.matchID",
		Other: "ゲーム終了！ マッチID：`{{.MatchID}}`\n{{.Winners}}",
	},
		map[string]interface{}{
			"MatchID": matchIDCode(dgs.ConnectCode, dgs.MatchID),
			"Winners": winners,
		})

	var footer *discordgo.MessageEmbedFooter

	if sett.DeleteGameSummaryMinutes > 0 {
		footer = &discordgo.MessageEmbedFooter{
			Text: sett.LocalizeMessage(&i18n.Message{
				ID:    "eventHandler.gameOver.deleteMessageFooter",
				Other: "このメッセージは{{.Mins}}分後に削除されます。",
			},
				map[string]interface{}{
					"Mins": sett.DeleteGameSummaryMinutes,
				}),
			IconURL:      "",
			ProxyIconURL: "",
		}
	}

	msg := discordgo.MessageEmbed{
		URL:         "",
		Type:        "",
		Title:       sett.LocalizeMessage(amongus.ToLocale(game.GAMEOVER)),
		Description: desc,
		Timestamp:   time.Now().Format(ISO8601),
		Footer:      footer,
		Color:       discord.DARK_GOLD, // DARK GOLD
		Image:       nil,
		Thumbnail:   getThumbnailFromMap(playMap, sett),
		Video:       nil,
		Provider:    nil,
		Author:      nil,
		Fields:      listResp,
	}
	return &msg
}

// 地図サムネは出さないようにする
func getThumbnailFromMap(playMap game.PlayMap, sett *settings.GuildSettings) *discordgo.MessageEmbedThumbnail {
	return nil
}

// ===== ゲーム中（TASK / DISCUSS） =====

func gamePlayMessage(dgs *GameState, emojis AlivenessEmojis, sett *settings.GuildSettings) *discordgo.MessageEmbed {
	phase := dgs.GameData.GetPhase()
	// send empty fields because we don't need to display those fields during the game...
	listResp := dgs.ToEmojiEmbedFields(emojis, sett)

	gameInfoFields := lobbyMetaEmbedFields("", "", dgs.GameStateMsg.LeaderID, dgs.VoiceChannel, dgs.GameData.GetNumDetectedPlayers(), dgs.GetCountLinked(), sett)
	listResp = append(gameInfoFields, listResp...)
	desc, color := dgs.descriptionAndColor(sett)
	if color == discord.DEFAULT {
		switch phase {
		case game.TASKS:
			color = discord.BLUE
		case game.DISCUSS:
			color = discord.PURPLE
		}
	}

	// フェーズ名を日本語寄りに
	title := ""
	switch phase {
	case game.TASKS:
		title = "タスク中"
	case game.DISCUSS:
		title = "会議中"
	case game.GAMEOVER:
		title = "ゲーム終了"
	default:
		title = sett.LocalizeMessage(amongus.ToLocale(phase))
	}

	msg := discordgo.MessageEmbed{
		URL:         "",
		Type:        "",
		Title:       title,
		Description: desc,
		Timestamp:   time.Now().Format(ISO8601),
		Color:       color,
		Footer:      nil,
		Image:       nil,
		Thumbnail:   nil, // 地図サムネなし
		Video:       nil,
		Provider:    nil,
		Author:      nil,
		Fields:      listResp,
	}

	return &msg
}

// returns the description and color to use, based on the gamestate
// usage dictates DEFAULT should be overwritten by other state subsequently,
// whereas RED and DARK_ORANGE are error/flag values that should be passed on
func (dgs *GameState) descriptionAndColor(sett *settings.GuildSettings) (string, int) {
	if !dgs.Linked {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "responses.notLinked.Description",
			Other: "❌**オートミュートキャプチャーと未接続**❌",
		}), discord.RED // red
	} else if !dgs.Running {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "responses.makeDescription.GameNotRunning",
			Other: "\n⚠ **BOTは一時停止中です** ⚠\n\n",
		}), discord.DARK_ORANGE
	}
	return "\n", discord.DEFAULT
}

func nonPremiumSettingResponse(sett *settings.GuildSettings) string {
	return sett.LocalizeMessage(&i18n.Message{
		ID:    "responses.nonPremiumSetting.Desc",
		Other: "この設定はAutoMuteUsプレミアム専用です。",
	})
}
