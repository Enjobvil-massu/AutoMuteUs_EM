package setting

import (
	"os"
	"strings"

	"github.com/automuteus/automuteus/v8/pkg/locale"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

func FnLanguage(sett *settings.GuildSettings, args []string) (interface{}, bool) {
	s := GetSettingByName(Language)
	if sett == nil {
		return nil, false
	}
	if forced := strings.TrimSpace(os.Getenv("BOT_LANG")); forced != "" {
		if len(args) == 0 {
			return ConstructEmbedForSetting(forced, s, sett), false
		}
		return "この環境では表示言語が日本語に固定されています。", false
	}
	if len(args) == 0 {
		return ConstructEmbedForSetting(sett.GetLanguage(), s, sett), false
	}

	if len(args[0]) < 2 {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingLanguage.tooShort",
			Other: "言語コードが短すぎます。利用可能な言語コード：{{.Langs}}",
		},
			map[string]interface{}{
				"Langs": locale.GetBundle().LanguageTags(),
			}), false
	}

	if len(locale.GetBundle().LanguageTags()) < 2 {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingLanguage.notLoaded",
			Other: "言語ファイルが読み込まれていません：{{.Langs}}",
		},
			map[string]interface{}{
				"Langs": locale.GetBundle().LanguageTags(),
			}), false
	}

	langName := locale.GetLanguages()[args[0]]
	if langName == "" {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingLanguage.notFound",
			Other: "指定された言語が見つかりません。利用可能な言語コード：{{.Langs}}",
		},
			map[string]interface{}{
				"Langs": locale.GetBundle().LanguageTags(),
			}), false
	}

	sett.SetLanguage(args[0])
	// easy way to check translation completeness; if the "Language" field is still set to English
	if langName == "English" && args[0] != "en" {
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingLanguage.set.needsTranslations",
			Other: "表示言語を `{{.LangCode}}` に変更しましたが、翻訳が不完全な可能性があります。",
		},
			map[string]interface{}{
				"LangCode": args[0],
			}), true
	}

	return sett.LocalizeMessage(&i18n.Message{
		ID:    "settings.SettingLanguage.set",
		Other: "表示言語を `{{.LangCode}}` に変更しました。",
	},
		map[string]interface{}{
			"LangCode": args[0],
		}), true
}
