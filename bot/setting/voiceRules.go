package setting

import (
	"github.com/automuteus/automuteus/v8/pkg/game"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

func FnVoiceRules(sett *settings.GuildSettings, args []string) (interface{}, bool) {
	if sett == nil {
		return nil, false
	}

	// now for a bunch of input checking
	if len(args) < 3 {
		// User didn't pass enough args
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingVoiceRules.enoughArgs",
			Other: "ミュートの種類、ゲーム状態、生存状態、有効／無効を指定してください。",
		}), false
	}

	gamePhase := game.GetPhaseFromString(args[1])
	if gamePhase == game.UNINITIALIZED {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingVoiceRules.Phase.UNINITIALIZED",
			Other: "ゲーム状態 `{{.PhaseName}}` が正しくありません。",
		},
			map[string]interface{}{
				"PhaseName": localizedSettingValue(args[1], sett),
			}), false
	}

	if args[2] != "alive" && args[2] != "dead" {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingVoiceRules.neitherAliveDead",
			Other: "`{{.Arg}}` は有効な生存状態ではありません。",
		},
			map[string]interface{}{
				"Arg": localizedSettingValue(args[2], sett),
			}), false
	}

	oldValue := sett.GetVoiceRule(args[0] == "muted", gamePhase, args[2])

	if len(args) == 3 {
		// User was only querying
		if oldValue {
			return sett.LocalizeMessage(&i18n.Message{
				ID:    "settings.SettingVoiceRules.queryingCurrentlyOldValues",
				Other: "「{{.PhaseName}}」では、{{.PlayerGameState}}プレイヤーを{{.PlayerDiscordState}}にする設定です。",
			},
				map[string]interface{}{
					"PhaseName":          localizedSettingValue(args[1], sett),
					"PlayerGameState":    localizedSettingValue(args[2], sett),
					"PlayerDiscordState": localizedSettingValue(args[0], sett),
				}), false
		} else {
			return sett.LocalizeMessage(&i18n.Message{
				ID:    "settings.SettingVoiceRules.queryingCurrentlyValues",
				Other: "「{{.PhaseName}}」では、{{.PlayerGameState}}プレイヤーを{{.PlayerDiscordState}}にしない設定です。",
			},
				map[string]interface{}{
					"PhaseName":          localizedSettingValue(args[1], sett),
					"PlayerGameState":    localizedSettingValue(args[2], sett),
					"PlayerDiscordState": localizedSettingValue(args[0], sett),
				}), false
		}
	}
	newValue := args[3] == "true"

	if newValue == oldValue {
		if newValue {
			return sett.LocalizeMessage(&i18n.Message{
				ID:    "settings.SettingVoiceRules.queryingAlreadyValues",
				Other: "「{{.PhaseName}}」の{{.PlayerGameState}}プレイヤーは、すでに{{.PlayerDiscordState}}に設定されています。",
			},
				map[string]interface{}{
					"PhaseName":          localizedSettingValue(args[1], sett),
					"PlayerGameState":    localizedSettingValue(args[2], sett),
					"PlayerDiscordState": localizedSettingValue(args[0], sett),
				}), false
		} else {
			return sett.LocalizeMessage(&i18n.Message{
				ID:    "settings.SettingVoiceRules.queryingAlreadyUnValues",
				Other: "「{{.PhaseName}}」の{{.PlayerGameState}}プレイヤーは、すでに{{.PlayerDiscordState}}にしない設定です。",
			},
				map[string]interface{}{
					"PhaseName":          localizedSettingValue(args[1], sett),
					"PlayerGameState":    localizedSettingValue(args[2], sett),
					"PlayerDiscordState": localizedSettingValue(args[0], sett),
				}), false
		}
	}

	if args[0] == "muted" {
		sett.SetVoiceRule(true, gamePhase, args[2], newValue)
	} else {
		sett.SetVoiceRule(false, gamePhase, args[2], newValue)
	}

	if newValue {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingVoiceRules.setValues",
			Other: "「{{.PhaseName}}」の{{.PlayerGameState}}プレイヤーを{{.PlayerDiscordState}}にする設定へ変更しました。",
		},
			map[string]interface{}{
				"PhaseName":          localizedSettingValue(args[1], sett),
				"PlayerGameState":    localizedSettingValue(args[2], sett),
				"PlayerDiscordState": localizedSettingValue(args[0], sett),
			}), true
	} else {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingVoiceRules.setUnValues",
			Other: "「{{.PhaseName}}」の{{.PlayerGameState}}プレイヤーを{{.PlayerDiscordState}}にしない設定へ変更しました。",
		},
			map[string]interface{}{
				"PhaseName":          localizedSettingValue(args[1], sett),
				"PlayerGameState":    localizedSettingValue(args[2], sett),
				"PlayerDiscordState": localizedSettingValue(args[0], sett),
			}), true
	}
}
