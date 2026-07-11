package setting

import (
	"github.com/automuteus/automuteus/v8/pkg/discord"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

func FnPermissionRoleIDs(sett *settings.GuildSettings, args []string) (interface{}, bool) {
	s := GetSettingByName(RoleIDs)
	if sett == nil {
		return nil, false
	}
	oldRoleIDs := sett.GetPermissionRoleIDs()
	if len(args) == 0 || args[0] == View {
		adminRoleCount := len(oldRoleIDs) // caching for optimisation
		// ロールメンションを現在の表示言語に合う区切りで並べます
		if adminRoleCount == 0 {
			return ConstructEmbedForSetting(sett.LocalizeMessage(&i18n.Message{
				ID:    "settings.SettingPermissionRoleIDs.noRoleAdmins",
				Other: "操作許可ロールは設定されていません。",
			}), s, sett), false
		} else {
			separator := ", "
			lastSeparator := " and "
			if sett.GetLanguage() == "ja" {
				separator = "、"
				lastSeparator = "、"
			}
			listOfRoles := ""
			for index, ID := range oldRoleIDs {
				switch {
				case index == 0:
					listOfRoles += "<@&" + ID + ">"
				case index == adminRoleCount-1:
					listOfRoles += lastSeparator + "<@&" + ID + ">"
				default:
					listOfRoles += separator + "<@&" + ID + ">"
				}
			}
			return ConstructEmbedForSetting(listOfRoles, s, sett), false
		}
	}

	if args[0] != Clear && args[0] != "c" {
		roleName := args[0]
		ID, err := discord.ExtractRoleIDFromText(roleName)
		if err != nil {
			return sett.LocalizeMessage(&i18n.Message{
				ID:    "settings.SettingPermissionRoleIDs.notFound",
				Other: "指定されたロールを確認できませんでした。",
			}), false
		}

		if ID != "" && !contains(oldRoleIDs, ID) {
			sett.SetPermissionRoleIDs(append(oldRoleIDs, ID))
			return sett.LocalizeMessage(&i18n.Message{
				ID:    "settings.SettingPermissionRoleIDs.newBotOperator",
				Other: "指定されたロールを操作許可ロールへ追加しました。",
			}), true
		} else {
			return sett.LocalizeMessage(&i18n.Message{
				ID:    "settings.SettingPermissionRoleIDs.alreadyBotOperator",
				Other: "指定されたロールはすでに操作許可ロールです。",
			}), false
		}
	} else {
		sett.SetPermissionRoleIDs([]string{})
		return sett.LocalizeMessage(&i18n.Message{
			ID:    "settings.SettingPermissionRoleIDs.clearRoles",
			Other: "操作許可ロールの設定をすべて解除しました。",
		}), true
	}
}
