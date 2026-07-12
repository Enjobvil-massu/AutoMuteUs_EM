package bot

import (
	"errors"
	"testing"
)

func TestReleaseLockForUnavailableStateReleasesNilState(t *testing.T) {
	calls := 0
	released := releaseLockForUnavailableState(nil, func() error {
		calls++
		return nil
	})

	if !released {
		t.Fatal("expected nil state to be treated as unavailable")
	}
	if calls != 1 {
		t.Fatalf("release calls = %d, want 1", calls)
	}
}

func TestReleaseLockForUnavailableStateKeepsValidStateLocked(t *testing.T) {
	calls := 0
	released := releaseLockForUnavailableState(&GameState{}, func() error {
		calls++
		return nil
	})

	if released {
		t.Fatal("expected valid state to keep its lock")
	}
	if calls != 0 {
		t.Fatalf("release calls = %d, want 0", calls)
	}
}

func TestReleaseLockForUnavailableStateAllowsMissingRelease(t *testing.T) {
	if !releaseLockForUnavailableState(nil, nil) {
		t.Fatal("expected nil state to be treated as unavailable")
	}
}

func TestReleaseLockForUnavailableStateHandlesReleaseError(t *testing.T) {
	calls := 0
	released := releaseLockForUnavailableState(nil, func() error {
		calls++
		return errors.New("release failed")
	})

	if !released {
		t.Fatal("expected nil state to be treated as unavailable")
	}
	if calls != 1 {
		t.Fatalf("release calls = %d, want 1", calls)
	}
}
