package command

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestHostMuteCommandDefinition(t *testing.T) {
	if HostMute.Name != "hostmute" {
		t.Fatalf("name = %q, want hostmute", HostMute.Name)
	}

	if HostMute.Description != "🎙️ ホスト発言モードを切り替えます" {
		t.Fatalf("description = %q", HostMute.Description)
	}

	if len(HostMute.Options) != 1 {
		t.Fatalf("option count = %d, want 1", len(HostMute.Options))
	}

	option := HostMute.Options[0]
	if option == nil {
		t.Fatal("mode option is nil")
	}

	if option.Name != HostMuteModeOption {
		t.Fatalf("option name = %q, want %q", option.Name, HostMuteModeOption)
	}

	if option.Type != discordgo.ApplicationCommandOptionString {
		t.Fatalf("option type = %v, want string", option.Type)
	}

	if option.Required {
		t.Fatal("mode option must remain optional")
	}

	if len(option.Choices) != 2 {
		t.Fatalf("choice count = %d, want 2", len(option.Choices))
	}

	if option.Choices[0].Name != "ONにする" ||
		option.Choices[0].Value != HostMuteModeOn {
		t.Fatalf("unexpected ON choice: %#v", option.Choices[0])
	}

	if option.Choices[1].Name != "OFFにする" ||
		option.Choices[1].Value != HostMuteModeOff {
		t.Fatalf("unexpected OFF choice: %#v", option.Choices[1])
	}
}

func TestHostMuteModeParsing(t *testing.T) {
	tests := []struct {
		value     string
		wantMode  bool
		wantValid bool
	}{
		{
			value:     "on",
			wantMode:  true,
			wantValid: true,
		},
		{
			value:     "ON",
			wantMode:  true,
			wantValid: true,
		},
		{
			value:     " off ",
			wantMode:  false,
			wantValid: true,
		},
		{
			value:     "invalid",
			wantMode:  false,
			wantValid: false,
		},
		{
			value:     "",
			wantMode:  false,
			wantValid: false,
		},
	}

	for _, tt := range tests {
		mode, valid := ParseHostMuteMode(tt.value)
		if mode != tt.wantMode || valid != tt.wantValid {
			t.Fatalf(
				"ParseHostMuteMode(%q) = (%v, %v), want (%v, %v)",
				tt.value,
				mode,
				valid,
				tt.wantMode,
				tt.wantValid,
			)
		}
	}
}

func TestGetHostMuteMode(t *testing.T) {
	mode, provided := GetHostMuteMode(nil)
	if provided || mode != "" {
		t.Fatalf(
			"nil options = (%q, %v), want empty/not-provided",
			mode,
			provided,
		)
	}

	options := []*discordgo.ApplicationCommandInteractionDataOption{
		{
			Name:  HostMuteModeOption,
			Type:  discordgo.ApplicationCommandOptionString,
			Value: "ON",
		},
	}

	mode, provided = GetHostMuteMode(options)
	if !provided || mode != HostMuteModeOn {
		t.Fatalf(
			"explicit mode = (%q, %v), want (%q, true)",
			mode,
			provided,
			HostMuteModeOn,
		)
	}
}

func TestHostMuteIsRegisteredAndEnabled(t *testing.T) {
	allCount := 0
	allIndex := -1

	for index, registered := range All {
		if registered != nil && registered.Name == HostMute.Name {
			allCount++
			allIndex = index
		}
	}

	if allCount != 1 {
		t.Fatalf(
			"HostMute command.All count = %d, want 1",
			allCount,
		)
	}

	if allIndex <= 0 || allIndex >= len(All)-1 {
		t.Fatalf(
			"HostMute command.All index = %d, want interior entry",
			allIndex,
		)
	}

	if All[allIndex-1] == nil ||
		All[allIndex-1].Name != End.Name {
		t.Fatalf(
			"command before HostMute = %#v, want End",
			All[allIndex-1],
		)
	}

	if All[allIndex+1] == nil ||
		All[allIndex+1].Name != Link.Name {
		t.Fatalf(
			"command after HostMute = %#v, want Link",
			All[allIndex+1],
		)
	}

	enabled, exists := EnabledSlashCommands[HostMute.Name]
	if !exists {
		t.Fatal("hostmute missing from EnabledSlashCommands")
	}
	if !enabled {
		t.Fatal("hostmute is not enabled")
	}

	enabledCount := 0
	for _, registered := range EnabledCommands() {
		if registered != nil && registered.Name == HostMute.Name {
			enabledCount++
		}
	}

	if enabledCount != 1 {
		t.Fatalf(
			"EnabledCommands HostMute count = %d, want 1",
			enabledCount,
		)
	}
}

func TestHostMuteIsInHelpChoices(t *testing.T) {
	if len(Help.Options) != 1 || Help.Options[0] == nil {
		t.Fatalf(
			"unexpected Help option layout: %#v",
			Help.Options,
		)
	}

	option := Help.Options[0]

	if len(option.Choices) > 25 {
		t.Fatalf(
			"Help choices = %d, exceeds Discord limit",
			len(option.Choices),
		)
	}

	count := 0

	for _, choice := range option.Choices {
		if choice == nil {
			continue
		}

		if choice.Name == HostMute.Name &&
			choice.Value == HostMute.Name {
			count++
		}
	}

	if count != 1 {
		t.Fatalf(
			"HostMute help choice count = %d, want 1",
			count,
		)
	}
}
