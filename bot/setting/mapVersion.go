package setting

import (
	"fmt"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"strings"
)

func FnMapVersion(sett *settings.GuildSettings, args []string) (interface{}, bool) {
	s := GetSettingByName(MapVersion)
	if sett == nil {
		return nil, false
	}
	if len(args) == 0 {
		return ConstructEmbedForSetting(fmt.Sprintf("%t", sett.GetMapDetailed()), s, sett), false
	}

	val := strings.ToLower(args[0]) == "true"
	sett.SetMapDetailed(val)
	return sett.LocalizeMessage(&i18n.Message{
		ID:    "settings.SettingMapVersion.Success",
		Other: "詳細マップの使用を「{{.Arg}}」へ変更しました。",
	},
		map[string]interface{}{
			"Arg": localizedSettingValue(fmt.Sprintf("%t", val), sett),
		}), true
}
