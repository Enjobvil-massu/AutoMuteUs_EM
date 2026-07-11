package command

import (
	"fmt"
	"strings"

	"github.com/automuteus/automuteus/v8/pkg/discord"
	"github.com/automuteus/automuteus/v8/pkg/game"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

const (
	ISO8601               = "2006-01-02T15:04:05-0700"
	BasePremiumURL        = "https://automute.us/premium?guild="
	CaptureDownloadURL    = "https://capture.automute.us"
	DefaultMaxActiveGames = 150
)

// All is all slash commands for the bot, ordered to match the README
var All = []*discordgo.ApplicationCommand{
	&Help,
	&New,
	&Refresh,
	&Pause,
	&End,
	&Link,
	&Unlink,
	&Settings,
	&Privacy,
	&Info,
	&Map,
	&Stats,
	&Premium,
	&Debug,
	&Download,
}

// ===== スラッシュコマンド有効・無効設定 =====
// true のものだけ Discord に登録されます。
// 一時的に隠したいコマンドは false にするだけで OK（後で true に戻せます）。
var EnabledSlashCommands = map[string]bool{
	"help":     true,
	"start":    true,
	"refresh":  false,
	"pause":    false,
	"stop":     true,
	"link":     true,
	"unlink":   true,
	"settings": true,
	"privacy":  false,
	"info":     false,
	"map":      false,
	// ↓ たぶん不要そうなものはデフォルトで off
	"stats":    false,
	"premium":  false,
	"debug":    false,
	"download": false,
}

// EnabledCommands は EnabledSlashCommands で true のコマンドだけを返します。
// main.go 側から、Discord へ登録する際はこの関数の結果を使います。
func EnabledCommands() []*discordgo.ApplicationCommand {
	var list []*discordgo.ApplicationCommand

	for _, cmd := range All {
		if enabled, ok := EnabledSlashCommands[cmd.Name]; ok && enabled {
			list = append(list, cmd)
		}
	}

	return list
}

func DeadlockGameStateResponse(command string, sett *settings.GuildSettings) *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: 1 << 6,
			Content: sett.LocalizeMessage(&i18n.Message{
				ID:    "commands.deadlock",
				Other: "コマンド {{.Command}} に必要なゲーム状態を取得できませんでした。もう一度お試しください。",
			}, map[string]interface{}{
				"Command": command,
			}),
		},
	}
}

func InsufficientPermissionsResponse(sett *settings.GuildSettings) *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: 1 << 6,
			Content: sett.LocalizeMessage(&i18n.Message{
				ID:    "commands.no_permissions",
				Other: "このコマンドを実行する権限がありません。",
			}),
		},
	}
}

func getCommand(cmd string) *discordgo.ApplicationCommand {
	for _, v := range All {
		if v.Name == cmd {
			return v
		}
	}
	return nil
}

func localizeCommandDescription(cmd *discordgo.ApplicationCommand, sett *settings.GuildSettings) string {
	return sett.LocalizeMessage(&i18n.Message{
		ID:    fmt.Sprintf("commands.%s.description", cmd.Name),
		Other: cmd.Description,
	})
}

// TODO supplement these embed with more detail than just the command description
func constructEmbedForCommand(
	cmd *discordgo.ApplicationCommand,
	sett *settings.GuildSettings,
) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		URL:         "",
		Type:        "",
		Title:       fmt.Sprintf("`/%s`", cmd.Name),
		Description: localizeCommandDescription(cmd, sett),
		Timestamp:   "",
		Color:       15844367, // GOLD
		Image:       nil,
		Thumbnail:   nil,
		Video:       nil,
		Provider:    nil,
		Author:      nil,
		Fields:      nil,
	}
}

var japaneseColorChoices = []struct {
	Value string
	Label string
}{
	{Value: "red", Label: "レッド"},
	{Value: "blue", Label: "ブルー"},
	{Value: "green", Label: "グリーン"},
	{Value: "pink", Label: "ピンク"},
	{Value: "orange", Label: "オレンジ"},
	{Value: "yellow", Label: "イエロー"},
	{Value: "black", Label: "ブラック"},
	{Value: "white", Label: "ホワイト"},
	{Value: "purple", Label: "パープル"},
	{Value: "brown", Label: "ブラウン"},
	{Value: "cyan", Label: "シアン"},
	{Value: "lime", Label: "ライム"},
	{Value: "maroon", Label: "マルーン"},
	{Value: "rose", Label: "ローズ"},
	{Value: "banana", Label: "バナナ"},
	{Value: "gray", Label: "グレー"},
	{Value: "tan", Label: "タン"},
	{Value: "coral", Label: "コーラル"},
}

// JapaneseColorName converts the internal English color value into the
// Japanese label shown to Discord users. Unknown values are preserved so an
// unexpected future color remains diagnosable instead of becoming blank.
func JapaneseColorName(color string) string {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(color), " ", ""))
	for _, choice := range japaneseColorChoices {
		if choice.Value == normalized {
			return choice.Label
		}
	}
	if strings.TrimSpace(color) == "" {
		return "不明"
	}
	return color
}

func colorsToCommandChoices() []*discordgo.ApplicationCommandOptionChoice {
	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0, len(japaneseColorChoices))
	for _, choice := range japaneseColorChoices {
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  choice.Label,
			Value: choice.Value,
		})
	}
	return choices
}

func mapsToCommandChoices() []*discordgo.ApplicationCommandOptionChoice {
	var choices []*discordgo.ApplicationCommandOptionChoice
	for mapValue, mapName := range game.MapNames {
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  mapName,
			Value: mapValue,
		})
	}
	return choices
}

func NoGameResponse(sett *settings.GuildSettings) *discordgo.InteractionResponse {
	return PrivateResponse(
		sett.LocalizeMessage(&i18n.Message{
			ID:    "commands.error.nogame",
			Other: "現在実行中のゲームはありません。",
		}))
}

func PrivateResponse(content string) *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   1 << 6,
			Content: content,
		},
	}
}

func PrivateErrorResponse(cmd string, err error, sett *settings.GuildSettings) *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: 1 << 6,
			Content: sett.LocalizeMessage(&i18n.Message{
				ID:    "commands.error",
				Other: "コマンド `{{.Command}}` の実行中にエラーが発生しました：`{{.Error}}`",
			}, map[string]interface{}{
				"Command": cmd,
				"Error":   err.Error(),
			}),
		},
	}
}

var PermissionStrings = map[int64]string{
	discordgo.PermissionViewChannel:        "チャンネルを見る",
	discordgo.PermissionSendMessages:       "メッセージを送信",
	discordgo.PermissionManageMessages:     "メッセージを管理",
	discordgo.PermissionEmbedLinks:         "埋め込みリンク",
	discordgo.PermissionUseExternalEmojis:  "外部絵文字を使用",
	discordgo.PermissionVoiceMuteMembers:   "メンバーをミュート",
	discordgo.PermissionVoiceDeafenMembers: "メンバーをスピーカーミュート",
}

func ReinviteMeResponse(missingPerms int64, channelID string, sett *settings.GuildSettings) *discordgo.InteractionResponse {
	missingPermsText := ""
	for v, str := range PermissionStrings {
		if v&missingPerms == v {
			missingPermsText += fmt.Sprintf("%s\n", str)
		}
	}
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: sett.LocalizeMessage(&i18n.Message{
				ID: "commands.error.reinvite",
				Other: "このサーバーまたはチャンネルで必要な権限が不足しています：\n```\n{{.Perm}}```\n" +
					"テキスト／ボイスチャンネル {{.Channel}} の権限を確認してください。必要に応じてBOTを再招待してください。",
			}, map[string]interface{}{
				"Perm":    missingPermsText,
				"Channel": discord.MentionByChannelID(channelID),
			}),
		},
	}
}
