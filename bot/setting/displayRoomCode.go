package setting

import (
	"fmt"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"strings"
)

func FnDisplayRoomCode(sett *settings.GuildSettings, args []string) (interface{}, bool) {
	s := GetSettingByName(DisplayRoomCode)
	if sett == nil {
		return nil, false
	}
	if len(args) == 0 {
		return ConstructEmbedForSetting(fmt.Sprintf("%v", sett.GetDisplayRoomCode()), s, sett), false
	}

	val := strings.ToLower(args[0])
	valid := map[string]bool{"always": true, "spoiler": true, "never": true}
	if !valid[val] {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingDisplayRoomCode.Unrecognized",
			Other: "`{{.Arg}}` は有効な表示方法ではありません。",
		},
			map[string]interface{}{
				"Arg": localizedSettingValue(val, sett),
			}), false
	}

	sett.SetDisplayRoomCode(val)
	if val == "spoiler" {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingDisplayRoomCode.Spoiler",
			Other: "ルームコードをスポイラー表示に変更しました。",
		},
			map[string]interface{}{
				"Arg": localizedSettingValue(val, sett),
			}), true
	} else {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingDisplayRoomCode.AlwaysOrNever",
			Other: "ルームコードの表示方法を「{{.Arg}}」へ変更しました。",
		},
			map[string]interface{}{
				"Arg": localizedSettingValue(val, sett),
			}), true
	}
}
