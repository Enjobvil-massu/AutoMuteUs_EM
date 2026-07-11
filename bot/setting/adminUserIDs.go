package setting

import (
	"github.com/automuteus/automuteus/v8/pkg/discord"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

func FnAdminUserIDs(sett *settings.GuildSettings, args []string) (interface{}, bool) {
	s := GetSettingByName(AdminUserIDs)
	if sett == nil {
		return nil, false
	}
	adminIDs := sett.GetAdminUserIDs()
	if len(args) == 0 || args[0] == View {
		adminCount := len(adminIDs) // caching for optimisation
		// Discordメンションを現在の表示言語に合う区切りで並べます
		if adminCount == 0 {
			return ConstructEmbedForSetting(sett.LocalizeMessage(&i18n.Message{
				ID:    "settings.SettingAdminUserIDs.noBotAdmins",
				Other: "BOT管理者は設定されていません。",
			}), s, sett), false
		} else {
			separator := ", "
			lastSeparator := " and "
			if sett.GetLanguage() == "ja" {
				separator = "、"
				lastSeparator = "、"
			}
			listOfAdmins := ""
			for index, ID := range adminIDs {
				switch {
				case index == 0:
					listOfAdmins += discord.MentionByUserID(ID)
				case index == adminCount-1:
					listOfAdmins += lastSeparator + discord.MentionByUserID(ID)
				default:
					listOfAdmins += separator + discord.MentionByUserID(ID)
				}
			}
			return ConstructEmbedForSetting(listOfAdmins, s, sett), false
		}
	}

	if args[0] != Clear && args[0] != "c" {
		userName := args[0]
		ID, err := discord.ExtractUserIDFromText(userName)
		if ID == "" || err != nil {
			return sett.LocalizeMessage(&i18n.Message{
				ID:    "settings.SettingAdminUserIDs.notFound",
				Other: "`{{.UserName}}` を特定できません。ユーザーIDまたはメンションで指定してください。",
			},
				map[string]interface{}{
					"UserName": userName,
				}), false
		} else {
			oldIDs := sett.GetAdminUserIDs()
			if ID != "" && !contains(oldIDs, ID) {
				sett.SetAdminUserIDs(append(oldIDs, ID))
				return sett.LocalizeMessage(&i18n.Message{
					ID:    "settings.SettingAdminUserIDs.newBotAdmin",
					Other: "{{.User}} をBOT管理者に追加しました。",
				},
					map[string]interface{}{
						"User": discord.MentionByUserID(ID),
					}), true
			} else {
				return sett.LocalizeMessage(&i18n.Message{
					ID:    "settings.SettingAdminUserIDs.alreadyBotAdmin",
					Other: "{{.User}} はすでにBOT管理者です。",
				},
					map[string]interface{}{
						"User": discord.MentionByUserID(ID),
					}), false
			}
		}

	} else {
		sett.SetAdminUserIDs([]string{})
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingAdminUserIDs.clearAdmins",
			Other: "BOT管理者の設定をすべて解除しました。",
		}), true
	}
}

func contains(arr []string, elem string) bool {
	for _, v := range arr {
		if v == elem {
			return true
		}
	}
	return false
}
