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

func TestHostMuteStageAIsNotRegistered(t *testing.T) {
	for _, registered := range All {
		if registered != nil && registered.Name == HostMute.Name {
			t.Fatal("HostMute must not be added to command.All during stage 3C4A")
		}
	}

	if _, exists := EnabledSlashCommands[HostMute.Name]; exists {
		t.Fatal("hostmute must not be exposed in EnabledSlashCommands during stage 3C4A")
	}
}
