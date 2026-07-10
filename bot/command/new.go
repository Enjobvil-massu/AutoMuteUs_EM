package command

import (
	"fmt"
	"strings"

	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

type NewStatus int

const (
	NewSuccess NewStatus = iota
	NewNoVoiceChannel
	NewLockout
)

const amongUsCaptureDownloadURL = "https://github.com/automuteus/amonguscapture/releases/latest"

type NewInfo struct {
	Hyperlink    string
	MinimalURL   string
	ApiHyperlink string
	ConnectCode  string
	ActiveGames  int64
}

// /new → /start にリネーム済み
var New = discordgo.ApplicationCommand{
	Name:        "start",
	Description: "オートミュートを開始します",
}

func NewResponse(status NewStatus, info NewInfo, sett *settings.GuildSettings) *discordgo.InteractionResponse {
	var content string

	// /start の接続情報は実行者だけに表示します。
	flags := discordgo.MessageFlagsEphemeral

	switch status {
	case NewSuccess:
		host := strings.TrimSpace(info.MinimalURL)
		host = strings.TrimSuffix(host, ":443")

		launchText := "自動起動リンクを利用できません。\n下のホストとコードを AmongUsCapture へ手動入力してください。"
		if isHTTPURL(info.ApiHyperlink) {
			launchText = fmt.Sprintf(
				"[ここをクリックして AmongUsCapture を起動・接続する](%s)\n※AmongUsCapture が入っているPCでクリックしてください。",
				strings.TrimSpace(info.ApiHyperlink),
			)
		}

		// Embed内のコードブロックではDiscordのコピーアイコンが表示されないため、
		// 通常メッセージ本文に独立したコードブロックとして表示します。
		content = fmt.Sprintf(
			"✅ **AutoMuteUsを開始しました**\n\n"+
				"まずは自動起動リンクを試してください。\n"+
				"起動しない場合は、下のホストとコードを AmongUsCapture へ手動入力してください。\n\n"+
				"🔗 **自動起動**\n%s\n\n"+
				"🔗 **ホスト**\n```text\n%s\n```\n\n"+
				"🔗 **コード**\n```text\n%s\n```\n\n"+
				"接続後、AmongUsCapture がフリーズする場合があります。\n"+
				"その場合は AmongUsCapture を再起動し、再度【登録】ボタンを押してください。\n\n"+
				"**AmongUsCaptureを入れていない場合**\n"+
				"[最新版のAmongUsCaptureをダウンロード](%s)",
			launchText,
			host,
			strings.TrimSpace(info.ConnectCode),
			amongUsCaptureDownloadURL,
		)

	case NewNoVoiceChannel:
		content = sett.LocalizeMessage(&i18n.Message{
			ID:    "commands.new.nochannel",
			Other: "ゲームを開始する前に、ボイスチャンネルへ参加してください。",
		})

	case NewLockout:
		content = sett.LocalizeMessage(&i18n.Message{
			ID: "commands.new.lockout",
			Other: "現在、起動中のゲーム数が多いため、新しいゲームを開始できません。\n" +
				"数分待ってから、もう一度 /start を実行してください。\n" +
				"現在のゲーム数: {{.Games}}",
		}, map[string]interface{}{
			"Games": fmt.Sprintf("%d/%d", info.ActiveGames, DefaultMaxActiveGames),
		})

		// ロックアウト警告だけは公開メッセージにします。
		flags = discordgo.MessageFlags(0)
	}

	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   flags,
			Content: content,
		},
	}
}

func isHTTPURL(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://")
}
