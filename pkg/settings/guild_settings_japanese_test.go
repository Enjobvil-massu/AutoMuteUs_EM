package settings

import "testing"

func TestGetLanguageUsesBOTLangOverride(t *testing.T) {
	t.Setenv("BOT_LANG", "ja")
	sett := &GuildSettings{Language: "en"}
	if got := sett.GetLanguage(); got != "ja" {
		t.Fatalf("GetLanguage() = %q, want ja", got)
	}
}

func TestGetLanguageUsesStoredValueWithoutOverride(t *testing.T) {
	t.Setenv("BOT_LANG", "")
	sett := &GuildSettings{Language: "ja"}
	if got := sett.GetLanguage(); got != "ja" {
		t.Fatalf("GetLanguage() = %q, want ja", got)
	}
}
