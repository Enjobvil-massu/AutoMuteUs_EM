package bot

import (
	"reflect"
	"testing"
)

func TestPlanHostTalkVoiceBatchOnMixedMembers(t *testing.T) {
	members := []hostTalkVoiceMember{
		{
			UserID:                "host",
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
			NormalMute:            true,
			NormalDeaf:            true,
		},
		{
			UserID:                "alive-linked",
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
			NormalMute:            false,
			NormalDeaf:            true,
		},
		{
			UserID:                "dead-linked",
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
			NormalMute:            false,
			NormalDeaf:            false,
		},
		{
			UserID:                "unlinked-human",
			InTrackedVoiceChannel: true,
			NormalApplicable:      false,
		},
		{
			UserID:                "discord-bot",
			IsBot:                 true,
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
			NormalMute:            true,
			NormalDeaf:            true,
			WasHostTalkManaged:    true,
		},
		{
			UserID:                "outside-unmanaged",
			InTrackedVoiceChannel: false,
			NormalApplicable:      false,
		},
		{
			UserID:                "outside-managed",
			InTrackedVoiceChannel: false,
			NormalApplicable:      false,
			WasHostTalkManaged:    true,
		},
	}

	want := []hostTalkVoicePlan{
		{
			UserID:              "host",
			Mute:                false,
			Deaf:                false,
			ManagedAfterSuccess: true,
		},
		{
			UserID:              "alive-linked",
			Mute:                true,
			Deaf:                false,
			ManagedAfterSuccess: true,
		},
		{
			UserID:              "dead-linked",
			Mute:                true,
			Deaf:                false,
			ManagedAfterSuccess: true,
		},
		{
			UserID:              "unlinked-human",
			Mute:                true,
			Deaf:                false,
			ManagedAfterSuccess: true,
		},
		{
			UserID:              "outside-managed",
			Mute:                false,
			Deaf:                false,
			ManagedAfterSuccess: false,
		},
	}

	got := planHostTalkVoiceBatch(true, "host", members)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("planHostTalkVoiceBatch(ON) = %#v, want %#v", got, want)
	}
}

func TestPlanHostTalkVoiceBatchOffRestoresNormalAndCleansUnlinked(t *testing.T) {
	members := []hostTalkVoiceMember{
		{
			UserID:                "host",
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
			NormalMute:            true,
			NormalDeaf:            true,
			WasHostTalkManaged:    true,
		},
		{
			UserID:                "managed-unlinked",
			InTrackedVoiceChannel: true,
			NormalApplicable:      false,
			WasHostTalkManaged:    true,
		},
		{
			UserID:                "normal-linked",
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
			NormalMute:            false,
			NormalDeaf:            true,
		},
		{
			UserID:                "normal-unlinked",
			InTrackedVoiceChannel: true,
			NormalApplicable:      false,
		},
		{
			UserID:                "managed-bot",
			IsBot:                 true,
			InTrackedVoiceChannel: true,
			WasHostTalkManaged:    true,
		},
	}

	want := []hostTalkVoicePlan{
		{
			UserID:              "host",
			Mute:                true,
			Deaf:                true,
			ManagedAfterSuccess: false,
		},
		{
			UserID:              "managed-unlinked",
			Mute:                false,
			Deaf:                false,
			ManagedAfterSuccess: false,
		},
		{
			UserID:              "normal-linked",
			Mute:                false,
			Deaf:                true,
			ManagedAfterSuccess: false,
		},
	}

	got := planHostTalkVoiceBatch(false, "host", members)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("planHostTalkVoiceBatch(OFF) = %#v, want %#v", got, want)
	}
}

func TestPlanHostTalkVoiceBatchMissingLeaderFallsBackToNormalRules(t *testing.T) {
	members := []hostTalkVoiceMember{
		{
			UserID:                "linked-human",
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
			NormalMute:            true,
			NormalDeaf:            false,
		},
		{
			UserID:                "unlinked-human",
			InTrackedVoiceChannel: true,
			NormalApplicable:      false,
		},
	}

	want := []hostTalkVoicePlan{
		{
			UserID: "linked-human",
			Mute:   true,
			Deaf:   false,
		},
	}

	got := planHostTalkVoiceBatch(true, "", members)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("missing-leader plan = %#v, want %#v", got, want)
	}
}

func TestPlanHostTalkVoiceBatchDoesNotMutateInput(t *testing.T) {
	members := []hostTalkVoiceMember{
		{
			UserID:                "host",
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
			NormalMute:            true,
			NormalDeaf:            true,
			WasHostTalkManaged:    true,
		},
		{
			UserID:                "participant",
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
			NormalMute:            false,
			NormalDeaf:            true,
		},
	}

	before := append([]hostTalkVoiceMember(nil), members...)

	_ = planHostTalkVoiceBatch(true, "host", members)

	if !reflect.DeepEqual(members, before) {
		t.Fatalf("planner mutated input: got %#v want %#v", members, before)
	}
}

func TestPlanHostTalkVoiceBatchEmptyInput(t *testing.T) {
	got := planHostTalkVoiceBatch(true, "host", nil)

	if got == nil {
		t.Fatal("planner returned nil; want deterministic empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("empty batch length = %d, want 0", len(got))
	}
}
