package command

import (
	"github.com/automuteus/automuteus/v8/pkg/discord"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"strings"
)

type LinkStatus int

const (
	LinkSuccess LinkStatus = iota
	LinkNoPlayer
	LinkNoGameData
)

var Link = discordgo.ApplicationCommand{
	Name:        "link",
	Description: "DiscordユーザーをAmong Us内の色へ手動リンクします",
	Options: []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionUser,
			Name:        "user",
			Description: "リンクするDiscordユーザー",
			Required:    true,
		},
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        "color",
			Description: "Among Us内の色",
			Required:    true,
			Choices:     colorsToCommandChoices(),
		},
	},
}

func GetLinkParams(s *discordgo.Session, options []*discordgo.ApplicationCommandInteractionDataOption) (string, string) {
	return options[0].UserValue(s).ID, strings.ReplaceAll(strings.ToLower(options[1].StringValue()), " ", "")
}

func LinkResponse(status LinkStatus, userID, color string, sett *settings.GuildSettings) *discordgo.InteractionResponse {
	var content string
	switch status {
	case LinkSuccess:
		content = sett.LocalizeMessage(&i18n.Message{
			ID:    "commands.link.success",
			Other: "{{.UserMention}} を「{{.Color}}」のプレイヤーにリンクしました。",
		}, map[string]interface{}{
			"UserMention": discord.MentionByUserID(userID),
			"Color":       JapaneseColorName(color),
		})
	case LinkNoPlayer:
		content = sett.LocalizeMessage(&i18n.Message{
			ID:    "commands.link.noplayer",
			Other: "現在のゲームに {{.UserMention}} とリンクできるプレイヤーが見つかりませんでした。",
		}, map[string]interface{}{
			"UserMention": discord.MentionByUserID(userID),
		})
	case LinkNoGameData:
		content = sett.LocalizeMessage(&i18n.Message{
			ID:    "commands.link.nogamedata",
			Other: "「{{.Color}}」のプレイヤー情報が見つかりませんでした。",
		}, map[string]interface{}{
			"Color": JapaneseColorName(color),
		})
	}

	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   1 << 6,
			Content: content,
		},
	}
}
