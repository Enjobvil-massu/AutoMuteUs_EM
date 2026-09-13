package bot

import (
	"reflect"
	"testing"
)

func TestFilterHostTalkManagedVoiceMembers(t *testing.T) {
	input := []hostTalkVoiceMember{
		{
			UserID:             "managed-one",
			WasHostTalkManaged: true,
		},
		{
			UserID:             "normal-user",
			WasHostTalkManaged: false,
		},
		{
			UserID:             "managed-two",
			WasHostTalkManaged: true,
		},
	}

	before := append([]hostTalkVoiceMember(nil), input...)

	got := filterHostTalkManagedVoiceMembers(input)

	want := []hostTalkVoiceMember{
		input[0],
		input[2],
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered = %#v, want %#v", got, want)
	}

	if !reflect.DeepEqual(input, before) {
		t.Fatalf("input mutated: got %#v, want %#v", input, before)
	}
}

func TestPlanHostTalkManagedOffBatchRestoresNormalRules(t *testing.T) {
	members := []hostTalkVoiceMember{
		{
			UserID:                "linked-managed",
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
			NormalMute:            true,
			NormalDeaf:            true,
			WasHostTalkManaged:    true,
		},
		{
			UserID:                "unlinked-managed",
			InTrackedVoiceChannel: true,
			NormalApplicable:      false,
			NormalMute:            false,
			NormalDeaf:            false,
			WasHostTalkManaged:    true,
		},
		{
			UserID:                "spectator-managed",
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
			NormalMute:            true,
			NormalDeaf:            false,
			WasHostTalkManaged:    true,
		},
		{
			UserID:                "unmanaged-normal",
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
			NormalMute:            true,
			NormalDeaf:            true,
			WasHostTalkManaged:    false,
		},
	}

	managed := filterHostTalkManagedVoiceMembers(members)
	got := planHostTalkVoiceBatch(false, "host", managed)

	want := []hostTalkVoicePlan{
		{
			UserID:              "linked-managed",
			Mute:                true,
			Deaf:                true,
			ManagedAfterSuccess: false,
		},
		{
			UserID:              "unlinked-managed",
			Mute:                false,
			Deaf:                false,
			ManagedAfterSuccess: false,
		},
		{
			UserID:              "spectator-managed",
			Mute:                true,
			Deaf:                false,
			ManagedAfterSuccess: false,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("off reconciliation plans = %#v, want %#v", got, want)
	}
}

func TestPlanHostTalkManagedMissingLeaderCleanup(t *testing.T) {
	members := []hostTalkVoiceMember{
		{
			UserID:                "managed",
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
			NormalMute:            false,
			NormalDeaf:            true,
			WasHostTalkManaged:    true,
		},
	}

	got := planHostTalkVoiceBatch(
		true,
		"",
		filterHostTalkManagedVoiceMembers(members),
	)

	want := []hostTalkVoicePlan{
		{
			UserID:              "managed",
			Mute:                false,
			Deaf:                true,
			ManagedAfterSuccess: false,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("missing-leader cleanup = %#v, want %#v", got, want)
	}
}

func TestPlanHostTalkManagedOffOutsideVCUnlinkedCleansToUnmuted(t *testing.T) {
	members := []hostTalkVoiceMember{
		{
			UserID:                "unlinked-outside",
			InTrackedVoiceChannel: false,
			NormalApplicable:      false,
			WasHostTalkManaged:    true,
		},
	}

	got := planHostTalkVoiceBatch(
		false,
		"host",
		filterHostTalkManagedVoiceMembers(members),
	)

	want := []hostTalkVoicePlan{
		{
			UserID:              "unlinked-outside",
			Mute:                false,
			Deaf:                false,
			ManagedAfterSuccess: false,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("outside cleanup = %#v, want %#v", got, want)
	}
}
