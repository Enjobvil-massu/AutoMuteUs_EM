package setting

import (
	"github.com/automuteus/automuteus/v8/pkg/game"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"strconv"
)

func FnDelays(sett *settings.GuildSettings, args []string) (interface{}, bool) {
	if sett == nil {
		return nil, false
	}
	// User passes phase name, phase name and new delay value
	if len(args) < 2 {
		// User didn't pass 2 phases, tell them the list of game phases
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingDelays.missingPhases",
			Other: "変更前と変更後のゲーム状態を指定してください。",
		}), false // find a better wording for this at some point
	}
	// now to find the actual game state from the string they passed
	var gamePhase1 = game.GetPhaseFromString(args[0])
	var gamePhase2 = game.GetPhaseFromString(args[1])
	if gamePhase1 == game.UNINITIALIZED {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingDelays.Phase.UNINITIALIZED",
			Other: "ゲーム状態 `{{.PhaseName}}` が正しくありません。",
		},
			map[string]interface{}{
				"PhaseName": localizedSettingValue(args[0], sett),
			}), false
	} else if gamePhase2 == game.UNINITIALIZED {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingDelays.Phase.UNINITIALIZED",
			Other: "ゲーム状態 `{{.PhaseName}}` が正しくありません。",
		},
			map[string]interface{}{
				"PhaseName": localizedSettingValue(args[1], sett),
			}), false
	}

	oldDelay := sett.GetDelay(gamePhase1, gamePhase2)
	if len(args) == 2 {
		// no number was passed, User was querying the delay
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingDelays.delayBetweenPhases",
			Other: "「{{.PhaseA}}」から「{{.PhaseB}}」へ変わる際の遅延は現在{{.OldDelay}}秒です。",
		},
			map[string]interface{}{
				"PhaseA":   localizedSettingValue(args[0], sett),
				"PhaseB":   localizedSettingValue(args[1], sett),
				"OldDelay": oldDelay,
			}), false
	}

	newDelay, err := strconv.Atoi(args[2])
	if err != nil || newDelay < 0 {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingDelays.wrongNumber",
			Other: "`{{.Number}}` は有効な秒数ではありません。",
		},
			map[string]interface{}{
				"Number": args[2],
			}), false
	}

	sett.SetDelay(gamePhase1, gamePhase2, newDelay)
	return sett.LocalizeMessage(&i18n.Message{
		ID:    "settings.SettingDelays.setDelayBetweenPhases",
		Other: "「{{.PhaseA}}」から「{{.PhaseB}}」へ変わる際の遅延を{{.OldDelay}}秒から{{.NewDelay}}秒へ変更しました。",
	},
		map[string]interface{}{
			"PhaseA":   localizedSettingValue(args[0], sett),
			"PhaseB":   localizedSettingValue(args[1], sett),
			"OldDelay": oldDelay,
			"NewDelay": newDelay,
		}), true
}
