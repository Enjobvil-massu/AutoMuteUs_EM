package bot

import "testing"

func TestPlanHostTalkVoiceEvent(t *testing.T) {
	tests := []struct {
		name  string
		input hostTalkVoiceEventInput
		want  hostTalkVoiceEventPlan
	}{
		{
			name: "on tracked host stays unmuted",
			input: hostTalkVoiceEventInput{
				HostTalkMode:          true,
				LeaderID:              "host",
				UserID:                "host",
				MemberKnown:           true,
				InTrackedVoiceChannel: true,
				NormalApplicable:      true,
				NormalMute:            true,
				NormalDeaf:            true,
			},
			want: hostTalkVoiceEventPlan{
				Handled:                       true,
				Apply:                         true,
				Mute:                          false,
				Deaf:                          false,
				ManagedAfterSuccess:           true,
				ExpectedInTrackedVoiceChannel: true,
			},
		},
		{
			name: "on tracked participant is muted without deaf",
			input: hostTalkVoiceEventInput{
				HostTalkMode:          true,
				LeaderID:              "host",
				UserID:                "participant",
				MemberKnown:           true,
				InTrackedVoiceChannel: true,
				NormalApplicable:      true,
				NormalMute:            false,
				NormalDeaf:            true,
			},
			want: hostTalkVoiceEventPlan{
				Handled:                       true,
				Apply:                         true,
				Mute:                          true,
				Deaf:                          false,
				ManagedAfterSuccess:           true,
				ExpectedInTrackedVoiceChannel: true,
			},
		},
		{
			name: "on tracked unlinked human is still managed",
			input: hostTalkVoiceEventInput{
				HostTalkMode:          true,
				LeaderID:              "host",
				UserID:                "unlinked",
				MemberKnown:           true,
				InTrackedVoiceChannel: true,
				NormalApplicable:      false,
			},
			want: hostTalkVoiceEventPlan{
				Handled:                       true,
				Apply:                         true,
				Mute:                          true,
				Deaf:                          false,
				ManagedAfterSuccess:           true,
				ExpectedInTrackedVoiceChannel: true,
			},
		},
		{
			name: "on tracked bot is owned but excluded fail closed",
			input: hostTalkVoiceEventInput{
				HostTalkMode:          true,
				LeaderID:              "host",
				UserID:                "music-bot",
				MemberKnown:           true,
				IsBot:                 true,
				InTrackedVoiceChannel: true,
				NormalApplicable:      true,
				NormalMute:            true,
				NormalDeaf:            true,
			},
			want: hostTalkVoiceEventPlan{
				Handled:                       true,
				ExpectedInTrackedVoiceChannel: true,
			},
		},
		{
			name: "on tracked unknown member is owned but excluded fail closed",
			input: hostTalkVoiceEventInput{
				HostTalkMode:          true,
				LeaderID:              "host",
				UserID:                "unknown",
				MemberKnown:           false,
				InTrackedVoiceChannel: true,
				NormalApplicable:      true,
				NormalMute:            true,
			},
			want: hostTalkVoiceEventPlan{
				Handled:                       true,
				ExpectedInTrackedVoiceChannel: true,
			},
		},
		{
			name: "on outside unmanaged user falls through to normal automute",
			input: hostTalkVoiceEventInput{
				HostTalkMode:          true,
				LeaderID:              "host",
				UserID:                "outside",
				MemberKnown:           true,
				InTrackedVoiceChannel: false,
				NormalApplicable:      true,
				NormalMute:            true,
				NormalDeaf:            true,
			},
			want: hostTalkVoiceEventPlan{},
		},
		{
			name: "on managed user leaving tracked vc is cleaned to normal state",
			input: hostTalkVoiceEventInput{
				HostTalkMode:          true,
				LeaderID:              "host",
				UserID:                "linked",
				MemberKnown:           true,
				InTrackedVoiceChannel: false,
				NormalApplicable:      true,
				NormalMute:            false,
				NormalDeaf:            false,
				WasHostTalkManaged:    true,
			},
			want: hostTalkVoiceEventPlan{
				Handled:                       true,
				Apply:                         true,
				Mute:                          false,
				Deaf:                          false,
				ManagedAfterSuccess:           false,
				ExpectedInTrackedVoiceChannel: false,
			},
		},
		{
			name: "on managed unlinked user leaving is explicitly unmuted",
			input: hostTalkVoiceEventInput{
				HostTalkMode:          true,
				LeaderID:              "host",
				UserID:                "unlinked",
				MemberKnown:           true,
				InTrackedVoiceChannel: false,
				NormalApplicable:      false,
				WasHostTalkManaged:    true,
			},
			want: hostTalkVoiceEventPlan{
				Handled:                       true,
				Apply:                         true,
				Mute:                          false,
				Deaf:                          false,
				ManagedAfterSuccess:           false,
				ExpectedInTrackedVoiceChannel: false,
			},
		},
		{
			name: "off managed linked user restores normal rules",
			input: hostTalkVoiceEventInput{
				HostTalkMode:          false,
				LeaderID:              "host",
				UserID:                "linked",
				MemberKnown:           true,
				InTrackedVoiceChannel: true,
				NormalApplicable:      true,
				NormalMute:            true,
				NormalDeaf:            true,
				WasHostTalkManaged:    true,
			},
			want: hostTalkVoiceEventPlan{
				Handled:                       true,
				Apply:                         true,
				Mute:                          true,
				Deaf:                          true,
				ManagedAfterSuccess:           false,
				ExpectedInTrackedVoiceChannel: true,
			},
		},
		{
			name: "off managed unlinked user cleans to unmuted",
			input: hostTalkVoiceEventInput{
				HostTalkMode:          false,
				LeaderID:              "host",
				UserID:                "unlinked",
				MemberKnown:           true,
				InTrackedVoiceChannel: true,
				NormalApplicable:      false,
				WasHostTalkManaged:    true,
			},
			want: hostTalkVoiceEventPlan{
				Handled:                       true,
				Apply:                         true,
				Mute:                          false,
				Deaf:                          false,
				ManagedAfterSuccess:           false,
				ExpectedInTrackedVoiceChannel: true,
			},
		},
		{
			name: "off unmanaged user stays on existing normal event path",
			input: hostTalkVoiceEventInput{
				HostTalkMode:          false,
				LeaderID:              "host",
				UserID:                "linked",
				MemberKnown:           true,
				InTrackedVoiceChannel: true,
				NormalApplicable:      true,
				NormalMute:            true,
			},
			want: hostTalkVoiceEventPlan{},
		},
		{
			name: "on missing leader unmanaged tracked user uses normal fallback",
			input: hostTalkVoiceEventInput{
				HostTalkMode:          true,
				LeaderID:              "",
				UserID:                "linked",
				MemberKnown:           true,
				InTrackedVoiceChannel: true,
				NormalApplicable:      true,
				NormalMute:            true,
			},
			want: hostTalkVoiceEventPlan{},
		},
		{
			name: "on missing leader managed user still cleans up",
			input: hostTalkVoiceEventInput{
				HostTalkMode:          true,
				LeaderID:              "",
				UserID:                "managed",
				MemberKnown:           true,
				InTrackedVoiceChannel: false,
				NormalApplicable:      false,
				WasHostTalkManaged:    true,
			},
			want: hostTalkVoiceEventPlan{
				Handled:                       true,
				Apply:                         true,
				Mute:                          false,
				Deaf:                          false,
				ManagedAfterSuccess:           false,
				ExpectedInTrackedVoiceChannel: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planHostTalkVoiceEvent(tt.input)

			if got != tt.want {
				t.Fatalf(
					"planHostTalkVoiceEvent() = %#v, want %#v",
					got,
					tt.want,
				)
			}
		})
	}
}

func TestPlanHostTalkVoiceEventDoesNotMutateInput(t *testing.T) {
	input := hostTalkVoiceEventInput{
		HostTalkMode:          true,
		LeaderID:              "host",
		UserID:                "participant",
		MemberKnown:           true,
		InTrackedVoiceChannel: true,
		NormalApplicable:      true,
		NormalMute:            false,
		NormalDeaf:            true,
		WasHostTalkManaged:    false,
	}

	before := input

	_ = planHostTalkVoiceEvent(input)

	if input != before {
		t.Fatalf("input mutated: got %#v, want %#v", input, before)
	}
}
