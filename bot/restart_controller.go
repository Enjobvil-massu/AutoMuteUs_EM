package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/automuteus/automuteus/v8/bot/command"
	"github.com/bwmarrin/discordgo"
)

type restartControllerRequest struct {
	Mode              string `json:"mode"`
	ApplicationID     string `json:"application_id"`
	InteractionToken  string `json:"interaction_token"`
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

func (bot *Bot) requestRestart(i *discordgo.InteractionCreate, mode string) *discordgo.InteractionResponse {
	if !hasDiscordAdministrator(bot.PrimarySession, i) {
		return command.PrivateResponse("❌ このコマンドはDiscordの「管理者」権限を持つメンバーのみ実行できます。")
	}

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("RESTART_CONTROLLER_URL")), "/")
	secret := strings.TrimSpace(os.Getenv("RESTART_CONTROLLER_TOKEN"))
	if baseURL == "" || secret == "" {
		return command.PrivateResponse("❌ 再起動コントローラーが設定されていません。管理者に確認してください。")
	}

	payload, err := json.Marshal(restartControllerRequest{
		Mode:             mode,
		ApplicationID:    i.ApplicationID,
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
