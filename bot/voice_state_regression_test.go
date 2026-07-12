package bot

import (
	"reflect"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestVoiceStateGameChannels(t *testing.T) {
	tests := []struct {
		name string
		m    *discordgo.VoiceStateUpdate
		want []string
	}{
		{
			name: "join tracked channel",
			m: &discordgo.VoiceStateUpdate{
				VoiceState: &discordgo.VoiceState{ChannelID: "new"},
			},
			want: []string{"new"},
		},
		{
			name: "leave tracked channel",
			m: &discordgo.VoiceStateUpdate{
				VoiceState:   &discordgo.VoiceState{},
				BeforeUpdate: &discordgo.VoiceState{ChannelID: "old"},
			},
			want: []string{"old"},
		},
		{
			name: "move between channels",
			m: &discordgo.VoiceStateUpdate{
				VoiceState:   &discordgo.VoiceState{ChannelID: "new"},
				BeforeUpdate: &discordgo.VoiceState{ChannelID: "old"},
			},
			want: []string{"old", "new"},
		},
		{
			name: "mute update in same channel is deduplicated",
			m: &discordgo.VoiceStateUpdate{
				VoiceState:   &discordgo.VoiceState{ChannelID: "same"},
				BeforeUpdate: &discordgo.VoiceState{ChannelID: "same"},
			},
			want: []string{"same"},
		},
		{
			name: "nil event",
			m:    nil,
			want: nil,
		},
		{
			name: "nil voice state",
			m:    &discordgo.VoiceStateUpdate{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := voiceStateGameChannels(tt.m); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("voiceStateGameChannels() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVoiceStateNeedsDiscordUpdate(t *testing.T) {
	tests := []struct {
		name                            string
		found, desiredMute, desiredDeaf bool
		actualMute, actualDeaf          bool
		want                            bool
	}{
		{
			name:        "actual mute differs",
			found:       true,
			desiredMute: true,
			actualMute:  false,
			want:        true,
		},
		{
			name:        "actual deafen differs",
			found:       true,
			desiredDeaf: true,
			actualDeaf:  false,
			want:        true,
		},
		{
			name:        "already correct",
			found:       true,
			desiredMute: true,
			desiredDeaf: true,
			actualMute:  true,
			actualDeaf:  true,
			want:        false,
		},
		{
			name:        "unlinked user",
			found:       false,
			desiredMute: true,
			actualMute:  false,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := voiceStateNeedsDiscordUpdate(
				tt.found,
				tt.desiredMute,
				tt.desiredDeaf,
				tt.actualMute,
				tt.actualDeaf,
			)
			if got != tt.want {
				t.Fatalf("voiceStateNeedsDiscordUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}
