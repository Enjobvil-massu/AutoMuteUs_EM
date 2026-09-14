package bot

import (
	"errors"
	"reflect"
	"testing"

	"github.com/automuteus/automuteus/v8/pkg/game"
)

func newHostTalkRecordTestState() *GameState {
	state := NewDiscordGameState("guild-1")
	state.ConnectCode = "ABCDEFGH"
	state.Running = true
	state.GameData.Phase = game.TASKS
	state.GameStateMsg.HostTalkMode = true
	state.GameStateMsg.HostTalkRevision = 41
	state.GameStateMsg.HostTalkManagedUsers = map[string]bool{
		"old-managed": true,
	}
	state.UserData["host"] = UserData{
		User: User{
			UserID: "host",
		},
		ShouldBeMute: true,
		ShouldBeDeaf: true,
	}
	state.UserData["participant"] = UserData{
		User: User{
			UserID: "participant",
		},
		ShouldBeMute: false,
		ShouldBeDeaf: true,
	}
	state.UserData["old-managed"] = UserData{
		User: User{
			UserID: "old-managed",
		},
		ShouldBeMute: true,
		ShouldBeDeaf: false,
	}
	return state
}

func TestPlanHostTalkSuccessfulVoiceRecordUpdatesDetachedCandidate(t *testing.T) {
	current := newHostTalkRecordTestState()

	originalUsers := make(UserDataSet, len(current.UserData))
	for userID, userData := range current.UserData {
		originalUsers[userID] = userData
	}
	originalManaged := map[string]bool{
		"old-managed": true,
	}

	records := []hostTalkVoiceRecord{
		{
			UserID:              "host",
			Mute:                false,
			Deaf:                false,
			ManagedAfterSuccess: true,
		},
		{
			UserID:              "participant",
			Mute:                true,
			Deaf:                false,
			ManagedAfterSuccess: true,
		},
		{
			UserID:              "old-managed",
			Mute:                false,
			Deaf:                false,
			ManagedAfterSuccess: false,
		},
		{
			UserID:              "unlinked-not-cached",
			Mute:                true,
			Deaf:                false,
			ManagedAfterSuccess: true,
		},
	}

	candidate, changed, err := planHostTalkSuccessfulVoiceRecord(
		current,
		"guild-1",
		"ABCDEFGH",
		game.TASKS,
		true,
		41,
		records,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if candidate == current {
		t.Fatal("candidate aliases current")
	}

	host := candidate.UserData["host"]
	if host.ShouldBeMute || host.ShouldBeDeaf {
		t.Fatalf("host state = mute:%v deaf:%v, want false/false", host.ShouldBeMute, host.ShouldBeDeaf)
	}

	participant := candidate.UserData["participant"]
	if !participant.ShouldBeMute || participant.ShouldBeDeaf {
		t.Fatalf("participant state = mute:%v deaf:%v, want true/false", participant.ShouldBeMute, participant.ShouldBeDeaf)
	}

	oldManaged := candidate.UserData["old-managed"]
	if oldManaged.ShouldBeMute || oldManaged.ShouldBeDeaf {
		t.Fatalf("old-managed state = mute:%v deaf:%v, want false/false", oldManaged.ShouldBeMute, oldManaged.ShouldBeDeaf)
	}

	if !candidate.GameStateMsg.HostTalkManagedUsers["host"] {
		t.Fatal("host not recorded as managed")
	}
	if !candidate.GameStateMsg.HostTalkManagedUsers["participant"] {
		t.Fatal("participant not recorded as managed")
	}
	if !candidate.GameStateMsg.HostTalkManagedUsers["unlinked-not-cached"] {
		t.Fatal("uncached successfully-applied human not recorded as managed")
	}
	if candidate.GameStateMsg.HostTalkManagedUsers["old-managed"] {
		t.Fatal("old-managed entry was not cleaned")
	}

	if !reflect.DeepEqual(current.UserData, originalUsers) {
		t.Fatalf("current UserData mutated: %#v", current.UserData)
	}
	if !reflect.DeepEqual(current.GameStateMsg.HostTalkManagedUsers, originalManaged) {
		t.Fatalf("current managed map mutated: %#v", current.GameStateMsg.HostTalkManagedUsers)
	}
}

func TestPlanHostTalkSuccessfulVoiceRecordRejectsStalePhase(t *testing.T) {
	current := newHostTalkRecordTestState()

	candidate, changed, err := planHostTalkSuccessfulVoiceRecord(
		current,
		"guild-1",
		"ABCDEFGH",
		game.DISCUSS,
		true,
		41,
		[]hostTalkVoiceRecord{
			{
				UserID:              "participant",
				Mute:                true,
				ManagedAfterSuccess: true,
			},
		},
	)

	if !errors.Is(err, errHostTalkVoiceSnapshotStale) {
		t.Fatalf("error = %v, want stale sentinel", err)
	}
	if changed {
		t.Fatal("stale phase reported changed")
	}
	if candidate != current {
		t.Fatal("stale phase should return original state")
	}
}

func TestPlanHostTalkSuccessfulVoiceRecordRejectsStaleRevision(t *testing.T) {
	current := newHostTalkRecordTestState()

	candidate, changed, err := planHostTalkSuccessfulVoiceRecord(
		current,
		"guild-1",
		"ABCDEFGH",
		game.TASKS,
		true,
		40,
		nil,
	)

	if !errors.Is(err, errHostTalkVoiceSnapshotStale) {
		t.Fatalf("error = %v, want stale sentinel", err)
	}
	if changed {
		t.Fatal("stale revision reported changed")
	}
	if candidate != current {
		t.Fatal("stale revision should return original state")
	}
}

func TestPlanHostTalkSuccessfulVoiceRecordRejectsModeFlip(t *testing.T) {
	current := newHostTalkRecordTestState()

	_, changed, err := planHostTalkSuccessfulVoiceRecord(
		current,
		"guild-1",
		"ABCDEFGH",
		game.TASKS,
		false,
		41,
		nil,
	)

	if !errors.Is(err, errHostTalkVoiceSnapshotStale) {
		t.Fatalf("error = %v, want stale sentinel", err)
	}
	if changed {
		t.Fatal("mode flip reported changed")
	}
}

func TestPlanHostTalkSuccessfulVoiceRecordNoChangesSkipsCommit(t *testing.T) {
	current := newHostTalkRecordTestState()

	current.UserData["host"] = UserData{
		User: User{
			UserID: "host",
		},
		ShouldBeMute: false,
		ShouldBeDeaf: false,
	}
	current.GameStateMsg.HostTalkManagedUsers["host"] = true

	persistCalls := 0

	result, changed, err := commitHostTalkSuccessfulVoiceRecord(
		current,
		"guild-1",
		"ABCDEFGH",
		game.TASKS,
		true,
		41,
		[]hostTalkVoiceRecord{
			{
				UserID:              "host",
				Mute:                false,
				Deaf:                false,
				ManagedAfterSuccess: true,
			},
		},
		func(*GameState) error {
			persistCalls++
			return nil
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatal("identical record reported changed")
	}
	if result != current {
		t.Fatal("no-op should return original state")
	}
	if persistCalls != 0 {
		t.Fatalf("persist calls = %d, want 0", persistCalls)
	}
}

func TestCommitHostTalkSuccessfulVoiceRecordPersistsCandidate(t *testing.T) {
	current := newHostTalkRecordTestState()

	var persisted *GameState
	result, changed, err := commitHostTalkSuccessfulVoiceRecord(
		current,
		"guild-1",
		"ABCDEFGH",
		game.TASKS,
		true,
		41,
		[]hostTalkVoiceRecord{
			{
				UserID:              "participant",
				Mute:                true,
				Deaf:                false,
				ManagedAfterSuccess: true,
			},
		},
		func(candidate *GameState) error {
			persisted = candidate
			return nil
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if result == current {
		t.Fatal("result aliases original")
	}
	if persisted != result {
		t.Fatal("persisted candidate does not match returned result")
	}
	if !persisted.GameStateMsg.HostTalkManagedUsers["participant"] {
		t.Fatal("participant was not persisted as managed")
	}
}

func TestCommitHostTalkSuccessfulVoiceRecordFailurePreservesOriginal(t *testing.T) {
	current := newHostTalkRecordTestState()

	originalParticipant := current.UserData["participant"]
	originalManaged := map[string]bool{
		"old-managed": true,
	}

	persistErr := errors.New("simulated persistence failure")

	result, changed, err := commitHostTalkSuccessfulVoiceRecord(
		current,
		"guild-1",
		"ABCDEFGH",
		game.TASKS,
		true,
		41,
		[]hostTalkVoiceRecord{
			{
				UserID:              "participant",
				Mute:                true,
				Deaf:                false,
				ManagedAfterSuccess: true,
			},
		},
		func(*GameState) error {
			return persistErr
		},
	)

	if !errors.Is(err, errHostTalkPersistFailed) {
		t.Fatalf("error = %v, want persist sentinel", err)
	}
	if !errors.Is(err, persistErr) {
		t.Fatalf("error = %v, want original persistence error", err)
	}
	if changed {
		t.Fatal("persistence failure reported changed")
	}
	if result != current {
		t.Fatal("persistence failure should return original")
	}
	if !reflect.DeepEqual(current.UserData["participant"], originalParticipant) {
		t.Fatal("persistence failure mutated original UserData")
	}
	if !reflect.DeepEqual(current.GameStateMsg.HostTalkManagedUsers, originalManaged) {
		t.Fatal("persistence failure mutated original managed map")
	}
}

func TestPlanHostTalkSuccessfulVoiceRecordEmptyUserIDIgnored(t *testing.T) {
	current := newHostTalkRecordTestState()

	candidate, changed, err := planHostTalkSuccessfulVoiceRecord(
		current,
		"guild-1",
		"ABCDEFGH",
		game.TASKS,
		true,
		41,
		[]hostTalkVoiceRecord{
			{
				UserID:              "",
				Mute:                true,
				Deaf:                true,
				ManagedAfterSuccess: true,
			},
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatal("empty user ID should not change state")
	}
	if candidate == current {
		t.Fatal("planner should still return detached candidate for a valid snapshot")
	}
}
