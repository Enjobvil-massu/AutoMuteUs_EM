package bot

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

const (
	hostTalkControlIDPrefix = "hosttalk-mode"

	hostTalkControlActionOn  = "on"
	hostTalkControlActionOff = "off"

	hostTalkStaleControlMessage = "このホスト発言モード操作画面は古いため使用できません。`/hostmute` をもう一度実行してください。"
	hostTalkUnauthorizedMessage = "この操作は現在のホスト、サーバー所有者、またはDiscordの管理者のみ実行できます。"
	hostTalkHostNotInVCMessage  = "ホストがゲームで使用中のボイスチャンネルに参加していないため、ホスト発言モードをONにできません。"
)

var errHostTalkControlIDInvalid = errors.New("invalid HostTalk control ID")

type hostTalkControl struct {
	ConnectCode          string
	GameCreationTimeUnix int64
	ExpectedRevision     uint64
	RequestedMode        bool
}

func hostTalkModeText(mode bool) string {
	if mode {
		return "ON"
	}
	return "OFF"
}

func hostTalkSelectorContent(mode bool) string {
	return fmt.Sprintf(
		"🎙️ ホスト発言モード\n現在の状態: **%s**\n切り替える状態を選択してください。",
		hostTalkModeText(mode),
	)
}

func buildHostTalkControlID(
	connectCode string,
	gameCreationTimeUnix int64,
	revision uint64,
	requestedMode bool,
) (string, error) {
	connectCode = strings.TrimSpace(connectCode)

	if connectCode == "" ||
		strings.Contains(connectCode, ":") ||
		gameCreationTimeUnix <= 0 {
		return "", errHostTalkControlIDInvalid
	}

	action := hostTalkControlActionOff
	if requestedMode {
		action = hostTalkControlActionOn
	}

	customID := fmt.Sprintf(
		"%s:%s:%d:%d:%s",
		hostTalkControlIDPrefix,
		connectCode,
		gameCreationTimeUnix,
		revision,
		action,
	)

	// Discord component custom_id is limited to 100 characters.
	// All generated protocol fields are ASCII, so byte length is a safe,
	// conservative upper bound here.
	if len(customID) > 100 {
		return "", errHostTalkControlIDInvalid
	}

	return customID, nil
}

func parseHostTalkControlID(customID string) (hostTalkControl, bool) {
	parts := strings.Split(customID, ":")
	if len(parts) != 5 ||
		parts[0] != hostTalkControlIDPrefix ||
		parts[1] == "" {
		return hostTalkControl{}, false
	}

	gameCreationTimeUnix, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || gameCreationTimeUnix <= 0 {
		return hostTalkControl{}, false
	}

	revision, err := strconv.ParseUint(parts[3], 10, 64)
	if err != nil {
		return hostTalkControl{}, false
	}

	var requestedMode bool
	switch parts[4] {
	case hostTalkControlActionOn:
		requestedMode = true
	case hostTalkControlActionOff:
		requestedMode = false
	default:
		return hostTalkControl{}, false
	}

	return hostTalkControl{
		ConnectCode:          parts[1],
		GameCreationTimeUnix: gameCreationTimeUnix,
		ExpectedRevision:     revision,
		RequestedMode:        requestedMode,
	}, true
}

func hostTalkSelectorComponents(
	connectCode string,
	gameCreationTimeUnix int64,
	revision uint64,
) ([]discordgo.MessageComponent, error) {
	onID, err := buildHostTalkControlID(
		connectCode,
		gameCreationTimeUnix,
		revision,
		true,
	)
	if err != nil {
		return nil, err
	}

	offID, err := buildHostTalkControlID(
		connectCode,
		gameCreationTimeUnix,
		revision,
		false,
	)
	if err != nil {
		return nil, err
	}

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					CustomID: onID,
					Style:    discordgo.SuccessButton,
					Label:    "ONにする",
					Emoji: discordgo.ComponentEmoji{
						Name: "🎙️",
					},
				},
				discordgo.Button{
					CustomID: offID,
					Style:    discordgo.DangerButton,
					Label:    "OFFにする",
				},
			},
		},
	}, nil
}

// hostTalkControlMatchesGame rejects a selector created for an older game,
// a refreshed/replaced game generation, or an older HostTalk revision.
func hostTalkControlMatchesGame(
	dgs *GameState,
	control hostTalkControl,
) bool {
	if dgs == nil ||
		control.ConnectCode == "" ||
		control.GameCreationTimeUnix <= 0 {
		return false
	}

	return strings.EqualFold(
		strings.TrimSpace(dgs.ConnectCode),
		strings.TrimSpace(control.ConnectCode),
	) &&
		dgs.GameStateMsg.CreationTimeUnix == control.GameCreationTimeUnix &&
		dgs.GameStateMsg.HostTalkRevision == control.ExpectedRevision
}

// hostTalkEnablePreflight implements the ON-only VC requirement.
// OFF intentionally has no corresponding requirement: an authorized actor
// must be able to turn HostTalk off even after the host leaves the tracked VC.
func hostTalkEnablePreflight(
	guild *discordgo.Guild,
	leaderID string,
	trackedVoiceChannelID string,
) bool {
	if guild == nil ||
		leaderID == "" ||
		trackedVoiceChannelID == "" {
		return false
	}

	for _, voiceState := range guild.VoiceStates {
		if voiceState == nil {
			continue
		}
		if voiceState.UserID == leaderID &&
			voiceState.ChannelID == trackedVoiceChannelID {
			return true
		}
	}

	return false
}
