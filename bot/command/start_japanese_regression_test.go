package command

import (
	"strings"
	"testing"

	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
)

func TestStartSuccessResponseAndBottomMessageAreJapanese(t *testing.T) {
	t.Setenv("BOT_LANG", "ja")
	resp := NewResponse(NewSuccess, NewInfo{
		MinimalURL:   "https://amu.enjobvil.com:443",
		ApiHyperlink: "https://amu.enjobvil.com/open/link?code=ABC123",
		ConnectCode:  "ABC123",
	}, &settings.GuildSettings{Language: "en"})

	if resp == nil || resp.Data == nil {
		t.Fatal("NewResponse returned nil")
	}
	if resp.Data.Flags != discordgo.MessageFlagsEphemeral {
		t.Fatalf("/start response flags = %v, want ephemeral", resp.Data.Flags)
	}

	required := []string{
		"AutoMuteUsを開始しました",
		"自動起動",
		"ホスト",
		"コード",
		"AmongUsCaptureを起動・接続する",
		"接続後に固まる場合は、AmongUsCaptureを再起動して再度【登録】を押してください。",
	}
	for _, text := range required {
		if !strings.Contains(resp.Data.Content, text) {
			t.Fatalf("/start response is missing %q: %q", text, resp.Data.Content)
		}
	}

	forbidden := []string{
		"Host URL",
		"Connection Code",
		"Successfully started",
		"Click here",
		"restart the capture",
	}
	for _, text := range forbidden {
		if strings.Contains(resp.Data.Content, text) {
			t.Fatalf("/start response contains English UI text %q: %q", text, resp.Data.Content)
		}
	}
}

func TestStartFallbackAndErrorsAreJapanese(t *testing.T) {
	t.Setenv("BOT_LANG", "ja")
	sett := &settings.GuildSettings{Language: "en"}

	tests := []struct {
		name   string
		status NewStatus
		info   NewInfo
		want   string
	}{
		{
			name:   "manual launch fallback",
			status: NewSuccess,
			info: NewInfo{
				MinimalURL:  "https://amu.enjobvil.com",
				ConnectCode: "ABC123",
			},
			want: "現在利用できません。下のホストとコードを手動入力してください。",
		},
		{
			name:   "not in voice channel",
			status: NewNoVoiceChannel,
			want:   "ゲームを開始する前に、ボイスチャンネルへ参加してください。",
		},
		{
			name:   "active game lockout",
			status: NewLockout,
			info:   NewInfo{ActiveGames: 3},
			want:   "現在、起動中のゲーム数が多いため、新しいゲームを開始できません。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := NewResponse(tt.status, tt.info, sett)
			if resp == nil || resp.Data == nil || !strings.Contains(resp.Data.Content, tt.want) {
				t.Fatalf("response was not Japanese: %#v", resp)
			}
		})
	}
}
