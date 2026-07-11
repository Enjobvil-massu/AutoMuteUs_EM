package setting

import (
	"fmt"
	"strings"

	"github.com/automuteus/automuteus/v8/pkg/game"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

const (
	MaxDelay = 10

	MaxLeaderBoardSize float64 = 10

	MaxLeaderBoardMin float64 = 100

	MaxMatchSummaryDelete float64 = 60

	View  = "view"
	Clear = "clear"
	User  = "user"
	Role  = "role"
)

var (
	MinDelay float64 = 0

	MinLeaderBoardSize float64 = 1

	MinLeaderBoardMin float64 = 1

	MinMatchSummaryDelete float64 = -1
)

const (
	Language            = "language"
	VoiceRules          = "voice-rules"
	AdminUserIDs        = "admin-user-ids"
	RoleIDs             = "operator-roles"
	UnmuteDead          = "unmute-dead"
	MapVersion          = "map-version"
	Delays              = "delays"
	MatchSummary        = "match-summary-duration"
	MatchSummaryChannel = "match-summary-channel"
	AutoRefresh         = "auto-refresh"
	LeaderboardMention  = "leaderboard-mention"
	LeaderboardSize     = "leaderboard-size"
	LeaderboardMin      = "leaderboard-min"
	MuteSpectators      = "mute-spectators"
	DisplayRoomCode     = "display-room-code"
	Show                = "show"
	List                = "list"
	Reset               = "reset"
)

func GetSettingByName(name string) *Setting {
	for _, v := range AllSettings {
		if v.Name == name {
			return &v
		}
	}
	return nil
}

func ToString(option *discordgo.ApplicationCommandInteractionDataOption) string {
	switch option.Type {
	case discordgo.ApplicationCommandOptionBoolean:
		return fmt.Sprintf("%t", option.BoolValue())
	case discordgo.ApplicationCommandOptionString:
		return option.StringValue()
	case discordgo.ApplicationCommandOptionInteger:
		return fmt.Sprintf("%d", option.IntValue())
	case discordgo.ApplicationCommandOptionUser:
		return option.UserValue(nil).Mention()
	case discordgo.ApplicationCommandOptionRole:
		return option.RoleValue(nil, "").Mention()
	case discordgo.ApplicationCommandOptionChannel:
		return option.ChannelValue(nil).Mention()
	case discordgo.ApplicationCommandOptionSubCommand:
		return option.Name
	default:
		return ""
	}
}

type Setting struct {
	Name      string
	ShortDesc string
	Arguments []*discordgo.ApplicationCommandOption
	Premium   bool
}

var phaseChoices = []*discordgo.ApplicationCommandOptionChoice{
	{
		Name:  "ロビー",
		Value: string(game.PhaseNames[game.LOBBY]),
	},
	{
		Name:  "タスク中",
		Value: string(game.PhaseNames[game.TASKS]),
	},
	{
		Name:  "会議中",
		Value: string(game.PhaseNames[game.DISCUSS]),
	},
}

var AllSettings = []Setting{
	{
		Name:      List,
		ShortDesc: "すべての設定を一覧表示します",
		Arguments: []*discordgo.ApplicationCommandOption{},
		Premium:   false,
	},
	{
		Name:      Language,
		ShortDesc: "BOTの表示言語を設定します",
		Arguments: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "language-code",
				Description: "言語コード（日本語は ja）",
			},
		},
		Premium: false,
	},
	{
		Name:      VoiceRules,
		ShortDesc: "各ゲーム状態でのミュート動作を設定します",
		Arguments: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "deaf-or-muted",
				Description: "ミュートの種類",
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{
						Name:  "スピーカーミュート",
						Value: "deafened",
					},
					{
						Name:  "マイクミュート",
						Value: "muted",
					},
				},
				Required: true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "phase",
				Description: "ゲーム状態",
				Choices:     phaseChoices,
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "alive",
				Description: "生存状態",
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{
						Name:  "生存",
						Value: "alive",
					},
					{
						Name:  "死亡",
						Value: "dead",
					},
				},
				Required: true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionBoolean,
				Name:        "value",
				Description: "有効／無効",
			},
		},
		Premium: false,
	},
	{
		Name:      AdminUserIDs,
		ShortDesc: "BOT管理者を設定します",
		Arguments: []*discordgo.ApplicationCommandOption{
			{
				Name:        View,
				Description: "BOT管理者を表示します",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
			{
				Name:        Clear,
				Description: "BOT管理者をすべて解除します",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        User,
				Description: "BOT管理者にするDiscordユーザー",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Name:        User,
						Description: "BOT管理者にするDiscordユーザー",
						Type:        discordgo.ApplicationCommandOptionUser,
						Required:    true,
					},
				},
			},
		},
		Premium: false,
	},
	{
		Name:      RoleIDs,
		ShortDesc: "BOT操作を許可するロールを設定します",
		Arguments: []*discordgo.ApplicationCommandOption{
			{
				Name:        View,
				Description: "操作許可ロールを表示します",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
			{
				Name:        Clear,
				Description: "操作許可ロールをすべて解除します",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        Role,
				Description: "BOT操作を許可するDiscordロール",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Name:        Role,
						Description: "BOT操作を許可するDiscordロール",
						Type:        discordgo.ApplicationCommandOptionRole,
						Required:    true,
					},
				},
			},
		},
		Premium: false,
	},
	{
		Name:      UnmuteDead,
		ShortDesc: "死亡者を直ちにミュート解除するか設定します",
		Arguments: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionBoolean,
				Name:        "unmute",
				Description: "死亡時に直ちに解除するか",
			},
		},
		Premium: false,
	},
	{
		Name:      MapVersion,
		ShortDesc: "マップ画像の詳細表示を設定します",
		Arguments: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionBoolean,
				Name:        "detailed",
				Description: "詳細マップを使用するか",
			},
		},
		Premium: false,
	},
	{
		Name:      Delays,
		ShortDesc: "ゲーム状態が変わる際のミュート遅延を設定します",
		Arguments: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "start-phase",
				Description: "変更前のゲーム状態",
				Choices:     phaseChoices,
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "end-phase",
				Description: "変更後のゲーム状態",
				Choices:     phaseChoices,
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "delay",
				Description: "遅延秒数",
				MinValue:    &MinDelay,
				MaxValue:    MaxDelay,
			},
		},
		Premium: false,
	},
	{
		Name:      MatchSummary,
		ShortDesc: "試合結果メッセージを残す時間を設定します",
		Arguments: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "minutes-duration",
				Description: "表示時間（分）",
				MinValue:    &MinMatchSummaryDelete,
				MaxValue:    MaxMatchSummaryDelete,
			},
		},
		Premium: true,
	},
	{
		Name:      MatchSummaryChannel,
		ShortDesc: "試合結果を投稿するチャンネルを設定します",
		Arguments: []*discordgo.ApplicationCommandOption{
			{
				Type:         discordgo.ApplicationCommandOptionChannel,
				Name:         "channel",
				Description:  "試合結果を投稿するテキストチャンネル",
				ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildText},
			},
		},
		Premium: true,
	},
	{
		Name:      AutoRefresh,
		ShortDesc: "状態メッセージの自動更新を設定します",
		Arguments: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionBoolean,
				Name:        "autorefresh",
				Description: "自動更新を有効にするか",
			},
		},
		Premium: true,
	},
	{
		Name:      LeaderboardMention,
		ShortDesc: "ランキングで参加者をメンションするか設定します",
		Arguments: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionBoolean,
				Name:        "use-mention",
				Description: "メンションを使用するか",
			},
		},
		Premium: true,
	},
	{
		Name:      LeaderboardSize,
		ShortDesc: "ランキングに表示する人数を設定します",
		Arguments: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "size",
				Description: "表示人数",
				MinValue:    &MinLeaderBoardSize,
				MaxValue:    MaxLeaderBoardSize,
			},
		},
		Premium: true,
	},
	{
		Name:      LeaderboardMin,
		ShortDesc: "ランキング掲載に必要な最低試合数を設定します",
		Arguments: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "minimum",
				Description: "最低試合数",
				MinValue:    &MinLeaderBoardMin,
				MaxValue:    MaxLeaderBoardMin,
			},
		},
		Premium: true,
	},
	{
		Name:      MuteSpectators,
		ShortDesc: "観戦者を死亡者と同様にミュートするか設定します",
		Arguments: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionBoolean,
				Name:        "mute",
				Description: "観戦者をミュートするか",
			},
		},
		Premium: true,
	},
	{
		Name:      DisplayRoomCode,
		ShortDesc: "ルームコードの表示方法を設定します",
		Arguments: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "visibility",
				Description: "表示方法",
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{
						Name:  "常に表示",
						Value: "always",
					},
					{
						Name:  "スポイラー表示",
						Value: "spoiler",
					},
					{
						Name:  "表示しない",
						Value: "never",
					},
				},
			},
		},
		Premium: true,
	},
	{
		Name:      Show,
		ShortDesc: "現在の設定をJSONで表示します",
		Arguments: []*discordgo.ApplicationCommandOption{},
		Premium:   false,
	},
	{
		Name:      Reset,
		ShortDesc: "BOT設定を初期値へ戻します",
		Arguments: []*discordgo.ApplicationCommandOption{},
		Premium:   false,
	},
}

var settingDisplayNames = map[string]string{
	List:                "設定一覧",
	Language:            "表示言語",
	VoiceRules:          "ミュート動作",
	AdminUserIDs:        "BOT管理者",
	RoleIDs:             "操作許可ロール",
	UnmuteDead:          "死亡者の即時解除",
	MapVersion:          "マップ表示",
	Delays:              "ミュート遅延",
	MatchSummary:        "試合結果の表示時間",
	MatchSummaryChannel: "試合結果チャンネル",
	AutoRefresh:         "状態メッセージ自動更新",
	LeaderboardMention:  "ランキングのメンション",
	LeaderboardSize:     "ランキング表示人数",
	LeaderboardMin:      "ランキング最低試合数",
	MuteSpectators:      "観戦者のミュート",
	DisplayRoomCode:     "ルームコード表示",
	Show:                "現在の設定",
	Reset:               "設定初期化",
}

func DisplayName(name string) string {
	if display, ok := settingDisplayNames[name]; ok {
		return display
	}
	return name
}

func displaySettingValue(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true":
		return "有効"
	case "false":
		return "無効"
	case "always":
		return "常に表示"
	case "spoiler":
		return "スポイラー表示"
	case "never":
		return "表示しない"
	case "simple":
		return "簡易"
	case "detailed":
		return "詳細"
	case "lobby":
		return "ロビー"
	case "tasks":
		return "タスク中"
	case "discussion":
		return "会議中"
	case "alive":
		return "生存"
	case "dead":
		return "死亡"
	case "muted":
		return "マイクミュート"
	case "deafened":
		return "スピーカーミュート"
	case "ja":
		return "日本語（ja）"
	case "en":
		return "英語（en）"
	default:
		return value
	}
}

func localizedSettingValue(value string, sett *settings.GuildSettings) string {
	if sett == nil || sett.GetLanguage() != "ja" {
		return value
	}
	return displaySettingValue(value)
}

func ConstructEmbedForSetting(value string, setting *Setting, sett *settings.GuildSettings) discordgo.MessageEmbed {
	if setting == nil {
		return discordgo.MessageEmbed{}
	}
	title := DisplayName(setting.Name)
	if setting.Premium {
		title = "💎 " + title
	}
	if value == "" {
		value = "null"
	}

	desc := sett.LocalizeMessage(&i18n.Message{
		ID:    "settings.ConstructEmbedForSetting.StarterDesc",
		Other: "`/settings {{.Command}}` でこの設定を表示・変更できます。\n\n",
	}, map[string]interface{}{
		"Command": setting.Name,
	})
	return discordgo.MessageEmbed{
		URL:         "",
		Type:        "",
		Title:       title,
		Description: desc + setting.ShortDesc,
		Timestamp:   "",
		Color:       15844367, // GOLD
		Image:       nil,
		Thumbnail:   nil,
		Video:       nil,
		Provider:    nil,
		Author:      nil,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: sett.LocalizeMessage(&i18n.Message{
					ID:    "settings.ConstructEmbedForSetting.Fields.CurrentValue",
					Other: "現在の設定値",
				}),
				Value:  localizedSettingValue(value, sett),
				Inline: false,
			},
		},
	}
}
