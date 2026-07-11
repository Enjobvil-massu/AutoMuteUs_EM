package setting

import (
	"fmt"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

func FnAutoRefresh(sett *settings.GuildSettings, args []string) (interface{}, bool) {
	s := GetSettingByName(AutoRefresh)
	if sett == nil {
		return nil, false
	}
	if len(args) == 0 {
		return ConstructEmbedForSetting(fmt.Sprintf("%v", sett.GetAutoRefresh()), s, sett), false
	}

	val := args[0]
	if val != "t" && val != "true" && val != "f" && val != "false" {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingAutoRefresh.Unrecognized",
			Other: "`{{.Arg}}` は有効な値ではありません。「有効」または「無効」を選択してください。",
		},
			map[string]interface{}{
				"Arg": val,
			}), false
	}

	newSet := val == "t" || val == "true"
	if sett.GetAutoRefresh() == newSet {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingAutoRefresh.Noop",
			Other: "状態メッセージの自動更新はすでに「{{.Value}}」です。",
		},
			map[string]interface{}{
				"Value": localizedSettingValue(fmt.Sprintf("%t", newSet), sett),
			}), false
	}
	sett.SetAutoRefresh(newSet)
	if newSet {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingAutoRefresh.True",
			Other: "状態メッセージの自動更新を有効にしました。",
		}), true
	} else {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingAutoRefresh.False",
			Other: "状態メッセージの自動更新を無効にしました。",
		}), true
	}
}
