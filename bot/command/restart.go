package command

import "github.com/bwmarrin/discordgo"

var administratorPermission = int64(discordgo.PermissionAdministrator)

var Restart = discordgo.ApplicationCommand{
	Name:                     "restart",
	Description:              "AutoMuteUsを再起動します（管理者専用）",
	DefaultMemberPermissions: &administratorPermission,
	DMPermission:             boolPtr(false),
}

var RestartAll = discordgo.ApplicationCommand{
	Name:                     "restart-all",
	Description:              "AutoMuteUsシステム全体を再起動します（管理者専用）",
	DefaultMemberPermissions: &administratorPermission,
	DMPermission:              boolPtr(false),
}

func boolPtr(v bool) *bool { return &v }
