package bot

import (
	"reflect"
	"testing"
)

func TestPlanHostTalkManagedVoiceReset(t *testing.T) {
	managed := map[string]bool{
		"tracked-human": true,
		"moved-human":   true,
		"managed-bot":   true,
		"not-present":   true,
		"false-entry":   false,
	}

	observations := map[string]hostTalkVoiceObservation{
		"tracked-human": {
			UserID:                "tracked-human",
			IsBot:                 false,
			InTrackedVoiceChannel: true,
			Mute:                  true,
			Deaf:                  false,
		},
		"moved-human": {
			UserID:                "moved-human",
			IsBot:                 false,
			InTrackedVoiceChannel: false,
			Mute:                  true,
			Deaf:                  true,
		},
		"managed-bot": {
			UserID:                "managed-bot",
			IsBot:                 true,
			InTrackedVoiceChannel: true,
			Mute:                  true,
			Deaf:                  true,
		},
		"unmanaged-human": {
			UserID:                "unmanaged-human",
			IsBot:                 false,
			InTrackedVoiceChannel: true,
			Mute:                  true,
			Deaf:                  true,
		},
		"false-entry": {
			UserID:                "false-entry",
			IsBot:                 false,
			InTrackedVoiceChannel: true,
			Mute:                  true,
			Deaf:                  true,
		},
	}

	managedBefore := map[string]bool{}
	for userID, value := range managed {
		managedBefore[userID] = value
	}

	observationsBefore := map[string]hostTalkVoiceObservation{}
	for userID, value := range observations {
		observationsBefore[userID] = value
	}

	got := planHostTalkManagedVoiceReset(
		managed,
		observations,
	)

	gotByID := map[string]hostTalkVoicePlan{}
	for _, plan := range got {
		if _, exists := gotByID[plan.UserID]; exists {
			t.Fatalf("duplicate reset plan for %q", plan.UserID)
		}
		gotByID[plan.UserID] = plan
	}

	if len(gotByID) != 2 {
		t.Fatalf(
			"reset plan count=%d, want 2: %#v",
			len(gotByID),
			gotByID,
		)
	}

	for _, userID := range []string{
		"tracked-human",
		"moved-human",
	} {
		plan, ok := gotByID[userID]
		if !ok {
			t.Fatalf(
				"missing managed human %q: %#v",
				userID,
				gotByID,
			)
		}

		if plan.Mute ||
			plan.Deaf ||
			plan.ManagedAfterSuccess {
			t.Fatalf(
				"unexpected reset plan for %q: %#v",
				userID,
				plan,
			)
		}
	}

	for _, userID := range []string{
		"managed-bot",
		"not-present",
		"unmanaged-human",
		"false-entry",
	} {
		if _, exists := gotByID[userID]; exists {
			t.Fatalf(
				"unexpected reset plan for %q: %#v",
				userID,
				gotByID[userID],
			)
		}
	}

	if !reflect.DeepEqual(managed, managedBefore) {
		t.Fatalf(
			"managed map mutated: got %#v, want %#v",
			managed,
			managedBefore,
		)
	}

	if !reflect.DeepEqual(
		observations,
		observationsBefore,
	) {
		t.Fatalf(
			"observation map mutated: got %#v, want %#v",
			observations,
			observationsBefore,
		)
	}
}

func TestPlanHostTalkManagedVoiceResetNilInputs(t *testing.T) {
	if got := planHostTalkManagedVoiceReset(nil, nil); len(got) != 0 {
		t.Fatalf("nil inputs produced plans: %#v", got)
	}
}
