package command

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

const (
	HostMuteModeOption = "mode"
	HostMuteModeOn     = "on"
	HostMuteModeOff    = "off"
)

var HostMute = discordgo.ApplicationCommand{
	Name:        "hostmute",
	Description: "🎙️ ホスト発言モードを切り替えます",
	Options: []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionString,
			Name:        HostMuteModeOption,
			Description: "ON/OFFを直接指定します（省略すると選択画面を表示）",
			Required:    false,
			Choices: []*discordgo.ApplicationCommandOptionChoice{
				{
					Name:  "ONにする",
					Value: HostMuteModeOn,
				},
				{
					Name:  "OFFにする",
					Value: HostMuteModeOff,
				},
			},
		},
	},
}

// GetHostMuteMode returns the explicit mode argument when one was supplied.
// No option means the caller should show the private ON/OFF selector.
func GetHostMuteMode(options []*discordgo.ApplicationCommandInteractionDataOption) (string, bool) {
	for _, option := range options {
		if option == nil || option.Name != HostMuteModeOption {
			continue
		}
		return strings.ToLower(strings.TrimSpace(option.StringValue())), true
	}
	return "", false
}

// ParseHostMuteMode converts the public command value into the boolean mode
// used by the HostTalk state transaction.
func ParseHostMuteMode(value string) (requestedMode bool, valid bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case HostMuteModeOn:
		return true, true
	case HostMuteModeOff:
		return false, true
	default:
		return false, false
	}
}
