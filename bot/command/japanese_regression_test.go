package command

import (
	"strings"
	"testing"
	"unicode"

	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
)

func containsJapaneseText(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) {
			return true
		}
	}
	return false
}

func requireJapaneseOptions(t *testing.T, commandName string, options []*discordgo.ApplicationCommandOption) {
	t.Helper()
	for _, option := range options {
		if strings.TrimSpace(option.Description) == "" {
			t.Fatalf("/%s option %q description is empty", commandName, option.Name)
		}
		if !containsJapaneseText(option.Description) {
			t.Fatalf("/%s option %q description is not Japanese: %q", commandName, option.Name, option.Description)
		}
		requireJapaneseOptions(t, commandName, option.Options)
	}
}

func TestJapaneseColorChoicesCoverAllColors(t *testing.T) {
	choices := colorsToCommandChoices()
	if len(choices) != 18 {
		t.Fatalf("colorsToCommandChoices() returned %d choices, want 18", len(choices))
	}
	if got := JapaneseColorName("Red"); got != "レッド" {
		t.Fatalf("JapaneseColorName(Red) = %q", got)
	}
	if got := JapaneseColorName("coral"); got != "コーラル" {
		t.Fatalf("JapaneseColorName(coral) = %q", got)
	}
	for _, choice := range choices {
		if !containsJapaneseText(choice.Name) {
			t.Fatalf("color choice is not Japanese: %#v", choice)
		}
	}
}

func TestEnabledCommandDescriptionsAreJapanese(t *testing.T) {
	commands := []*discordgo.ApplicationCommand{
		&New,
		&Link,
		&Unlink,
		&Help,
		&Settings,
	}
	for _, cmd := range commands {
		if strings.TrimSpace(cmd.Description) == "" {
			t.Fatalf("/%s description is empty", cmd.Name)
		}
		if !containsJapaneseText(cmd.Description) {
			t.Fatalf("/%s description is not Japanese: %q", cmd.Name, cmd.Description)
		}
		requireJapaneseOptions(t, cmd.Name, cmd.Options)
	}
}

func TestLinkResponseUsesJapaneseColor(t *testing.T) {
	t.Setenv("BOT_LANG", "ja")
	resp := LinkResponse(LinkSuccess, "123", "red", &settings.GuildSettings{Language: "en"})
	if resp == nil || resp.Data == nil || !strings.Contains(resp.Data.Content, "レッド") {
		t.Fatalf("link response was not Japanese: %#v", resp)
	}
	if strings.Contains(resp.Data.Content, "Successfully linked") {
		t.Fatalf("link response contains English fallback: %q", resp.Data.Content)
	}
}
