package setting

import (
	"strings"
	"testing"

	"github.com/automuteus/automuteus/v8/pkg/settings"
)

func TestLocalizedSettingValueJapanese(t *testing.T) {
	t.Setenv("BOT_LANG", "")
	sett := &settings.GuildSettings{Language: "ja"}

	tests := map[string]string{
		"true":       "有効",
		"false":      "無効",
		"Lobby":      "ロビー",
		"Tasks":      "タスク中",
		"Discussion": "会議中",
		"alive":      "生存",
		"dead":       "死亡",
		"muted":      "マイクミュート",
		"deafened":   "スピーカーミュート",
		"always":     "常に表示",
		"spoiler":    "スポイラー表示",
		"never":      "表示しない",
	}

	for input, want := range tests {
		if got := localizedSettingValue(input, sett); got != want {
			t.Fatalf("localizedSettingValue(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLocalizedSettingValuePreservesEnglishMode(t *testing.T) {
	t.Setenv("BOT_LANG", "")
	sett := &settings.GuildSettings{Language: "en"}
	if got := localizedSettingValue("Tasks", sett); got != "Tasks" {
		t.Fatalf("English mode changed the internal value: %q", got)
	}
}

func TestLanguageSettingExplainsForcedJapanese(t *testing.T) {
	t.Setenv("BOT_LANG", "ja")
	sett := &settings.GuildSettings{Language: "en"}
	response, changed := FnLanguage(sett, []string{"en"})
	if changed {
		t.Fatal("forced language setting must not report a persisted change")
	}
	message, ok := response.(string)
	if !ok || !strings.Contains(message, "日本語に固定") {
		t.Fatalf("unexpected forced language response: %#v", response)
	}
}
