package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/automuteus/automuteus/v8/bot/command"
	"github.com/automuteus/automuteus/v8/pkg/rediskey"
	"github.com/bwmarrin/discordgo"
)

const (
	restartConfirmIDPrefix = "restart-confirm"
	restartCancelIDPrefix  = "restart-cancel"
)

type restartControllerRequest struct {
	Mode             string `json:"mode"`
	ApplicationID    string `json:"application_id"`
	InteractionToken string `json:"interaction_token"`
}

func hasDiscordAdministrator(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	if i == nil || i.GuildID == "" || i.Member == nil || i.Member.User == nil {
		return false
	}
	perms, err := s.UserChannelPermissions(i.Member.User.ID, i.ChannelID)
	if err != nil {
		return false
	}
	return perms&discordgo.PermissionAdministrator != 0
}

func restartConfirmationComponents(userID, mode string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					CustomID: fmt.Sprintf("%s:%s:%s", restartConfirmIDPrefix, userID, mode),
					Style:    discordgo.DangerButton,
					Label:    "再起動する",
					Emoji:    discordgo.ComponentEmoji{Name: "🔄"},
				},
				discordgo.Button{
					CustomID: fmt.Sprintf("%s:%s:%s", restartCancelIDPrefix, userID, mode),
					Style:    discordgo.SecondaryButton,
					Label:    "キャンセル",
				},
			},
		},
	}
}

func restartWarningResponse(userID, mode string, activeGames int64) *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: 1 << 6,
			Content: fmt.Sprintf(
				"⚠️ 現在 AutoMuteUs を使用中のサーバーがあります。\n使用中のゲームセッション: **%d件**\n再起動すると進行中のAutoMuteUsセッションが中断される可能性があります。\n本当に再起動しますか？",
				activeGames,
			),
			Components: restartConfirmationComponents(userID, mode),
		},
	}
}

func (bot *Bot) requestRestart(i *discordgo.InteractionCreate, mode string, force bool) *discordgo.InteractionResponse {
	if !hasDiscordAdministrator(bot.PrimarySession, i) {
		return command.PrivateResponse("❌ このコマンドはDiscordの「管理者」権限を持つメンバーのみ実行できます。")
	}

	if mode != "bot" && mode != "all" {
		return command.PrivateResponse("❌ 再起動モードが不正です。")
	}

	if !force {
		activeGames := rediskey.GetActiveGames(context.Background(), bot.RedisInterface.client, GameTimeoutSeconds)
		if activeGames > 0 {
			return restartWarningResponse(i.Member.User.ID, mode, activeGames)
		}
	}

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("RESTART_CONTROLLER_URL")), "/")
	secret := strings.TrimSpace(os.Getenv("RESTART_CONTROLLER_TOKEN"))
	if baseURL == "" || secret == "" {
		return command.PrivateResponse("❌ 再起動コントローラーが設定されていません。管理者に確認してください。")
	}

	payload, err := json.Marshal(restartControllerRequest{
		Mode:             mode,
		ApplicationID:    i.AppID,
		InteractionToken: i.Token,
	})
	if err != nil {
		return command.PrivateResponse("❌ 再起動要求の作成に失敗しました。")
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/restart", bytes.NewReader(payload))
	if err != nil {
		return command.PrivateResponse("❌ 再起動要求の作成に失敗しました。")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+secret)

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return command.PrivateResponse(fmt.Sprintf("❌ 再起動コントローラーへ接続できません: %v", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return command.PrivateResponse(fmt.Sprintf("❌ 再起動要求が拒否されました（HTTP %d）。", resp.StatusCode))
	}

	if mode == "all" {
		return command.PrivateResponse("🔄 AutoMuteUs / Galactus / Redis / PostgreSQL の再起動を開始します。完了または失敗はこの操作への追跡メッセージで通知します。")
	}
	return command.PrivateResponse("🔄 AutoMuteUsの再起動を開始します。完了または失敗はこの操作への追跡メッセージで通知します。")
}

func (bot *Bot) handleRestartConfirmation(i *discordgo.InteractionCreate, customID string) *discordgo.InteractionResponse {
	parts := strings.SplitN(customID, ":", 3)
	if len(parts) != 3 || parts[1] == "" || (parts[2] != "bot" && parts[2] != "all") {
		return command.PrivateResponse("❌ この再起動確認は無効です。もう一度コマンドを実行してください。")
	}

	if i.Member == nil || i.Member.User == nil || i.Member.User.ID != parts[1] {
		return command.PrivateResponse("❌ この確認ボタンは、再起動コマンドを実行した管理者のみ使用できます。")
	}

	resp := bot.requestRestart(i, parts[2], true)
	if resp == nil || resp.Data == nil {
		return resp
	}

	// Replace the warning panel so its buttons cannot be pressed again.
	resp.Type = discordgo.InteractionResponseUpdateMessage
	resp.Data.Components = []discordgo.MessageComponent{}
	return resp
}

func restartCancelResponse(i *discordgo.InteractionCreate, customID string) *discordgo.InteractionResponse {
	parts := strings.SplitN(customID, ":", 3)
	if len(parts) != 3 || parts[1] == "" || (parts[2] != "bot" && parts[2] != "all") {
		return command.PrivateResponse("❌ この再起動確認は無効です。もう一度コマンドを実行してください。")
	}

	if i.Member == nil || i.Member.User == nil || i.Member.User.ID != parts[1] {
		return command.PrivateResponse("❌ この確認ボタンは、再起動コマンドを実行した管理者のみ使用できます。")
	}

	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Flags:      1 << 6,
			Content:    "再起動をキャンセルしました。",
			Components: []discordgo.MessageComponent{},
		},
	}
}
