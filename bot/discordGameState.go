package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/automuteus/automuteus/v8/pkg/amongus"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// GameState represents a full record of the entire current game's state. It is intended to be fully JSON-serializable,
// so that any shard/worker can pick up the game state and operate upon it (using locks as necessary)
type GameState struct {
	GuildID string `json:"guildID"`

	ConnectCode string `json:"connectCode"`

	Linked     bool `json:"linked"`
	Running    bool `json:"running"`
	Subscribed bool `json:"subscribed"`

	MatchID        int64 `json:"matchID"`
	MatchStartUnix int64 `json:"matchStartUnix"`

	UserData     UserDataSet       `json:"userData"`
	DisplayNames map[string]string `json:"displayNames"` // 追加: userID -> 表示名（ニックネーム優先）
	VoiceChannel string            `json:"voiceChannel"`
	GameStateMsg GameStateMessage  `json:"gameStateMessage"`
	GameData     amongus.GameData  `json:"amongUsData"`

	// ===== 追加: AmongUsCapture 接続状態 =====
	CaptureConnected bool  `json:"captureConnected"`
	LastCapturePing  int64 `json:"lastCapturePing,omitempty"`
}

// ===== GameState ヘルパー =====

func NewDiscordGameState(guildID string) *GameState {
	dgs := GameState{GuildID: guildID}
	dgs.Reset()
	return &dgs
}

func (dgs *GameState) Reset() {
	// Explicitly does not reset the GuildID!
	dgs.ConnectCode = ""
	dgs.Linked = false
	dgs.Running = false
	dgs.Subscribed = false
	dgs.MatchID = -1
	dgs.MatchStartUnix = -1
	dgs.UserData = map[string]UserData{}
	dgs.DisplayNames = map[string]string{} // 表示名キャッシュもリセット
	dgs.VoiceChannel = ""
	dgs.GameStateMsg = MakeGameStateMessage()
	dgs.GameData = amongus.NewGameData()

	// ===== 追加: Capture未接続で初期化 =====
	dgs.CaptureConnected = false
	dgs.LastCapturePing = 0
}

// discordGuildMemberPayload is intentionally kept local instead of upgrading discordgo.
// The current bot uses discordgo v0.27.1, whose User type does not include global_name.
// Reading the member JSON directly lets us use Discord's display name without changing
// component APIs or the current button layout.
type discordGuildMemberPayload struct {
	Nick string `json:"nick"`
	User struct {
		ID         string `json:"id"`
		Username   string `json:"username"`
		GlobalName string `json:"global_name"`
	} `json:"user"`
}

func chooseDiscordDisplayName(nick, globalName, username, userID string) string {
	for _, candidate := range []string{nick, globalName, username, userID} {
		if name := strings.TrimSpace(candidate); name != "" {
			return name
		}
	}
	return "不明なユーザー"
}

// resolveDiscordDisplayName follows Discord's visible-name priority:
// server nickname -> global display name -> account username -> user ID.
// discordgo v0.27.1 is intentionally retained for compatibility with the
// current component code, so global_name is read from Discord's REST JSON.
func resolveDiscordDisplayName(s *discordgo.Session, guildID, userID, nick, username string) string {
	if name := strings.TrimSpace(nick); name != "" {
		return name
	}

	if s != nil && guildID != "" && userID != "" {
		endpoint := discordgo.EndpointGuildMember(guildID, userID)
		body, err := s.RequestWithBucketID("GET", endpoint, nil, discordgo.EndpointGuildMembers(guildID))
		if err == nil {
			var payload discordGuildMemberPayload
			if err := json.Unmarshal(body, &payload); err == nil {
				return chooseDiscordDisplayName(payload.Nick, payload.User.GlobalName, payload.User.Username, userID)
			}
		} else {
			log.Printf("Unable to fetch Discord display name for user %s in guild %s: %v", userID, guildID, err)
		}
	}

	return chooseDiscordDisplayName(nick, "", username, userID)
}

func (dgs *GameState) cacheDisplayName(s *discordgo.Session, guildID, userID, nick, username string) {
	if dgs.DisplayNames == nil {
		dgs.DisplayNames = map[string]string{}
	}
	dgs.DisplayNames[userID] = resolveDiscordDisplayName(s, guildID, userID, nick, username)
}

// ギルドメンバー情報をキャッシュしつつ UserData を作成
func (dgs *GameState) checkCacheAndAddUser(g *discordgo.Guild, s *discordgo.Session, userID string) (UserData, bool) {
	if g == nil {
		return UserData{}, false
	}

	// ===== 1. Guild メンバーキャッシュから探す =====
	for _, m := range g.Members {
		if m.User != nil && m.User.ID == userID {
			user := MakeUserDataFromDiscordUser(m.User, m.Nick)
			dgs.UserData[m.User.ID] = user

			// サーバー内表示名 → Discord表示名 → アカウント名の順で保存
			dgs.cacheDisplayName(s, g.ID, m.User.ID, m.Nick, m.User.Username)

			return user, true
		}
	}

	// ===== 2. API で取得（キャッシュに無い場合） =====
	endpoint := discordgo.EndpointGuildMember(g.ID, userID)
	body, err := s.RequestWithBucketID(
		"GET",
		endpoint,
		nil,
		discordgo.EndpointGuildMembers(g.ID),
	)
	if err != nil {
		log.Println(err)
		return UserData{}, false
	}

	var payload discordGuildMemberPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Println(err)
		return UserData{}, false
	}

	// Discord応答にIDがない場合でも、要求時のIDをフォールバックに使用します。
	if payload.User.ID == "" {
		payload.User.ID = userID
	}

	discordUser := &discordgo.User{
		ID:       payload.User.ID,
		Username: payload.User.Username,
	}

	user := MakeUserDataFromDiscordUser(discordUser, payload.Nick)
	dgs.UserData[payload.User.ID] = user

	if dgs.DisplayNames == nil {
		dgs.DisplayNames = map[string]string{}
	}
	dgs.DisplayNames[payload.User.ID] = chooseDiscordDisplayName(
		payload.Nick,
		payload.User.GlobalName,
		payload.User.Username,
		payload.User.ID,
	)

	return user, true
}

//
// ===== ここからプレイヤー表示用の色ラベルヘルパー =====
//

// ボタンと同じ表記用の色マスタ
type colorLabelPattern struct {
	Key   string
	Label string
}

var colorLabelPatterns = []colorLabelPattern{
	{Key: "red", Label: "🟥 レッド"},
	{Key: "black", Label: "⬛ ブラック"},
	{Key: "white", Label: "⬜ ホワイト"},
	{Key: "rose", Label: "🌸 ローズ"},

	{Key: "blue", Label: "🔵 ブルー"},
	{Key: "cyan", Label: "🟦 シアン"},
	{Key: "yellow", Label: "🟨 イエロー"},
	{Key: "pink", Label: "💗 ピンク"},

	{Key: "purple", Label: "🟣 パープル"},
	{Key: "orange", Label: "🟧 オレンジ"},
	{Key: "banana", Label: "🍌 バナナ"},
	{Key: "coral", Label: "🧱 コーラル"},

	{Key: "lime", Label: "🥬 ライム"},
	{Key: "green", Label: "🌲 グリーン"},
	{Key: "gray", Label: "⬜ グレー"},
	{Key: "maroon", Label: "🍷 マルーン"},

	{Key: "brown", Label: "🤎 ブラウン"},
	{Key: "tan", Label: "🟫 タン"},
}

// Emoji 名（例: "AliveRed", "DeadBlue" など）から「🟥 レッド」形式を返す
func colorLabelFromEmojiName(name string) string {
	lower := strings.ToLower(name)
	for _, p := range colorLabelPatterns {
		if strings.Contains(lower, p.Key) {
			return p.Label
		}
	}
	// マッチしなかったときのフォールバック
	return "❓ 不明"
}

//
// ===== ここから Embed のプレイヤー一覧生成 =====
//

// ToEmojiEmbedFields はゲーム状態から Embed のフィールドを生成する
// ・各色ごとに 1 フィールド
// ・フィールド名: アモアス名（ディスコード表示名）
// ・フィールド本文: 状態 と 色の情報
func (dgs *GameState) ToEmojiEmbedFields(emojis AlivenessEmojis, sett *settings.GuildSettings) []*discordgo.MessageEmbedField {
	players := make([]amongus.PlayerData, 0, len(dgs.GameData.PlayerData))
	for _, player := range dgs.GameData.PlayerData {
		players = append(players, player)
	}

	return dgs.toEmojiEmbedFieldsForPlayers(players, emojis, sett)
}

func (dgs *GameState) toEmojiEmbedFieldsForPlayers(players []amongus.PlayerData, emojis AlivenessEmojis, sett *settings.GuildSettings) []*discordgo.MessageEmbedField {
	// 色順で並べるための一時配列（最大 18 色）
	unsorted := make([]*discordgo.MessageEmbedField, 18)
	num := 0

	for _, player := range players {
		if player.Color < 0 || player.Color > 17 {
			continue
		}

		// 生存/死亡で別のクルー絵文字を取得
		emoji := emojis[player.IsAlive][player.Color]

		// 状態テキスト（生存 / 死亡）
		statusText := "生存中"
		if !player.IsAlive {
			statusText = "死亡中"
		}

		// ボタンと同じ色表記（🟥 レッド など）
		colorLabel := colorLabelFromEmojiName(emoji.Name)

		field := &discordgo.MessageEmbedField{
			Inline: false, // 1人ずつ改行表示
		}

		linked := false
		for _, userData := range dgs.UserData {
			if userData.InGameName == player.Name {
				// リンク済みプレイヤー

				// userID からキャッシュしておいた表示名を取得
				userID := userData.GetID()
				displayName := ""
				if dgs.DisplayNames != nil {
					displayName = dgs.DisplayNames[userID]
				}
				// 古い保存データなどでキャッシュがない場合も、ID表示を避ける
				if displayName == "" {
					displayName = userData.GetNickName()
				}
				if displayName == "" {
					displayName = userData.GetUserName()
				}
				if displayName == "" {
					displayName = userID
				}

				// フィールド名：アモアス名（表示名） ※メンションではないのでピン通知されない
				field.Name = fmt.Sprintf("%s（%s）", player.Name, displayName)

				// 本文：状態：<クルー絵文字> 生存/死亡　色：🟥 レッド
				field.Value = fmt.Sprintf(
					"状態：%s %s　色：%s",
					emoji.FormatForInline(), // クルーの絵文字のみ（🟢 や 💀 は使わない）
					statusText,
					colorLabel,
				)

				linked = true
				break
			}
		}

		if !linked {
			// 未リンクプレイヤー
			unlinkedText := sett.LocalizeMessage(&i18n.Message{
				ID:    "discordGameState.ToEmojiEmbedFields.Unlinked",
				Other: "🚫 **未リンク**",
			})

			field.Name = fmt.Sprintf("%s（%s）", player.Name, unlinkedText)
			field.Value = fmt.Sprintf(
				"状態：%s %s　色：%s",
				emoji.FormatForInline(),
				statusText,
				colorLabel,
			)
		}

		unsorted[player.Color] = field
		num++
	}

	// 色順に並べ替え
	sorted := make([]*discordgo.MessageEmbedField, 0, num)
	for i := 0; i < 18; i++ {
		if unsorted[i] != nil {
			sorted = append(sorted, unsorted[i])
		}
	}

	// ※1人1ブロックで縦並びにするので、最後の行を埋めるパディングは不要
	return sorted
}
