package bot

import (
	"errors"
	"reflect"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestObserveHostTalkVoiceEventUser(t *testing.T) {
	guild := &discordgo.Guild{
		Members: []*discordgo.Member{
			{
				User: &discordgo.User{
					ID:  "human",
					Bot: false,
				},
			},
			{
				User: &discordgo.User{
					ID:  "bot",
					Bot: true,
				},
			},
			{
				User: &discordgo.User{
					ID:  "absent-human",
					Bot: false,
				},
			},
		},
		VoiceStates: []*discordgo.VoiceState{
			{
				UserID:    "human",
				ChannelID: "tracked",
				Mute:      true,
				Deaf:      false,
			},
			{
				UserID:    "bot",
				ChannelID: "tracked",
			},
			{
				UserID:    "resolved-human",
				ChannelID: "tracked",
				Mute:      false,
				Deaf:      true,
			},
		},
	}

	human := observeHostTalkVoiceEventUser(
		guild,
		"human",
		"tracked",
		nil,
	)
	if !human.MemberKnown ||
		human.IsBot ||
		!human.InVoice ||
		!human.InTrackedVoiceChannel ||
		!human.Mute ||
		human.Deaf {
		t.Fatalf("unexpected human observation: %#v", human)
	}

	botObservation := observeHostTalkVoiceEventUser(
		guild,
		"bot",
		"tracked",
		nil,
	)
	if !botObservation.MemberKnown || !botObservation.IsBot {
		t.Fatalf("bot identity not preserved: %#v", botObservation)
	}

	absent := observeHostTalkVoiceEventUser(
		guild,
		"absent-human",
		"tracked",
		nil,
	)
	if !absent.MemberKnown ||
		absent.IsBot ||
		absent.InVoice ||
		absent.InTrackedVoiceChannel {
		t.Fatalf("unexpected absent-human observation: %#v", absent)
	}

	resolved := observeHostTalkVoiceEventUser(
		guild,
		"resolved-human",
		"tracked",
		func(userID string) (*discordgo.Member, error) {
			if userID != "resolved-human" {
				t.Fatalf("unexpected resolver ID %q", userID)
			}
			return &discordgo.Member{
				User: &discordgo.User{
					ID:  userID,
					Bot: false,
				},
			}, nil
		},
	)
	if !resolved.MemberKnown ||
		resolved.IsBot ||
		!resolved.InTrackedVoiceChannel ||
		resolved.Mute ||
		!resolved.Deaf {
		t.Fatalf("resolver observation incorrect: %#v", resolved)
	}

	unknown := observeHostTalkVoiceEventUser(
		guild,
		"resolved-human",
		"tracked",
		func(string) (*discordgo.Member, error) {
			return nil, errors.New("simulated lookup failure")
		},
	)
	if unknown.MemberKnown {
		t.Fatalf("failed resolver must remain unknown: %#v", unknown)
	}
	if !unknown.InTrackedVoiceChannel {
		t.Fatalf("voice location should still be observable: %#v", unknown)
	}
}

func TestHostTalkVoiceEventObservationMatchesPlan(t *testing.T) {
	plan := hostTalkVoiceEventPlan{
		Handled:                       true,
		Apply:                         true,
		Mute:                          true,
		Deaf:                          false,
		ManagedAfterSuccess:           true,
		ExpectedInTrackedVoiceChannel: true,
	}

	valid := hostTalkVoiceEventObservation{
		MemberKnown:           true,
		IsBot:                 false,
		InVoice:               true,
		InTrackedVoiceChannel: true,
	}

	if !hostTalkVoiceEventObservationMatchesPlan(plan, valid) {
		t.Fatal("valid event observation was rejected")
	}

	botObservation := valid
	botObservation.IsBot = true
	if hostTalkVoiceEventObservationMatchesPlan(plan, botObservation) {
		t.Fatal("bot observation was accepted")
	}

	unknown := valid
	unknown.MemberKnown = false
	if hostTalkVoiceEventObservationMatchesPlan(plan, unknown) {
		t.Fatal("unknown member observation was accepted")
	}

	moved := valid
	moved.InTrackedVoiceChannel = false
	if hostTalkVoiceEventObservationMatchesPlan(plan, moved) {
		t.Fatal("VC-side change was accepted")
	}
}

func TestHostTalkVoiceEventNeedsDiscordUpdate(t *testing.T) {
	trackedPlan := hostTalkVoiceEventPlan{
		Handled:                       true,
		Apply:                         true,
		Mute:                          true,
		Deaf:                          false,
		ManagedAfterSuccess:           true,
		ExpectedInTrackedVoiceChannel: true,
	}

	different := hostTalkVoiceEventObservation{
		MemberKnown:           true,
		InVoice:               true,
		InTrackedVoiceChannel: true,
		Mute:                  false,
		Deaf:                  false,
	}

	if !hostTalkVoiceEventNeedsDiscordUpdate(trackedPlan, different) {
		t.Fatal("different tracked state should require Discord update")
	}

	alreadyApplied := different
	alreadyApplied.Mute = true
	if hostTalkVoiceEventNeedsDiscordUpdate(trackedPlan, alreadyApplied) {
		t.Fatal("already-applied state should not require Discord update")
	}

	outsideCleanup := hostTalkVoiceEventPlan{
		Handled:                       true,
		Apply:                         true,
		Mute:                          false,
		Deaf:                          false,
		ManagedAfterSuccess:           false,
		ExpectedInTrackedVoiceChannel: false,
	}

	leftVoice := hostTalkVoiceEventObservation{
		MemberKnown:           true,
		InVoice:               false,
		InTrackedVoiceChannel: false,
	}

	if hostTalkVoiceEventNeedsDiscordUpdate(outsideCleanup, leftVoice) {
		t.Fatal("user already outside voice should not require Discord request")
	}

	outsideStillMuted := hostTalkVoiceEventObservation{
		MemberKnown:           true,
		InVoice:               true,
		InTrackedVoiceChannel: false,
		Mute:                  true,
	}

	if !hostTalkVoiceEventNeedsDiscordUpdate(outsideCleanup, outsideStillMuted) {
		t.Fatal("muted user in another VC should require cleanup request")
	}
}

func TestHostTalkVoiceRecordForEventPlan(t *testing.T) {
	plan := hostTalkVoiceEventPlan{
		Handled:                       true,
		Apply:                         true,
		Mute:                          true,
		Deaf:                          false,
		ManagedAfterSuccess:           true,
		ExpectedInTrackedVoiceChannel: true,
	}

	got := hostTalkVoiceRecordForEventPlan(
		plan,
		"12345",
	)

	want := hostTalkVoiceRecord{
		UserID:                        "12345",
		Mute:                          true,
		Deaf:                          false,
		ManagedAfterSuccess:           true,
		ExpectedInTrackedVoiceChannel: true,
	}

	if got != want {
		t.Fatalf("record = %#v, want %#v", got, want)
	}
}

func TestVoiceStateGameChannelsHostTalkTransitions(t *testing.T) {
	tests := []struct {
		name string
		in   *discordgo.VoiceStateUpdate
		want []string
	}{
		{
			name: "join",
			in: &discordgo.VoiceStateUpdate{
				VoiceState: &discordgo.VoiceState{
					ChannelID: "tracked",
				},
			},
			want: []string{"tracked"},
		},
		{
			name: "leave",
			in: &discordgo.VoiceStateUpdate{
				VoiceState: &discordgo.VoiceState{},
				BeforeUpdate: &discordgo.VoiceState{
					ChannelID: "tracked",
				},
			},
			want: []string{"tracked"},
		},
		{
			name: "move",
			in: &discordgo.VoiceStateUpdate{
				VoiceState: &discordgo.VoiceState{
					ChannelID: "new",
				},
				BeforeUpdate: &discordgo.VoiceState{
					ChannelID: "old",
				},
			},
			want: []string{"old", "new"},
		},
		{
			name: "same channel deduplicated",
			in: &discordgo.VoiceStateUpdate{
				VoiceState: &discordgo.VoiceState{
					ChannelID: "tracked",
				},
				BeforeUpdate: &discordgo.VoiceState{
					ChannelID: "tracked",
				},
			},
			want: []string{"tracked"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := voiceStateGameChannels(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf(
					"voiceStateGameChannels() = %#v, want %#v",
					got,
					tt.want,
				)
			}
		})
	}
}
func TestHostTalkManagedUnlinkedSpectatorRestoresNormalRule(t *testing.T) {
	input := hostTalkVoiceEventInput{
		HostTalkMode:          false,
		LeaderID:              "host",
		UserID:                "unlinked-spectator",
		MemberKnown:           true,
		InTrackedVoiceChannel: true,
		NormalApplicable:      true,
		NormalMute:            true,
		NormalDeaf:            false,
		WasHostTalkManaged:    true,
	}

	got := planHostTalkVoiceEvent(input)

	want := hostTalkVoiceEventPlan{
		Handled:                       true,
		Apply:                         true,
		Mute:                          true,
		Deaf:                          false,
		ManagedAfterSuccess:           false,
		ExpectedInTrackedVoiceChannel: true,
	}

	if got != want {
		t.Fatalf(
			"managed unlinked spectator restore = %#v, want %#v",
			got,
			want,
		)
	}
}
