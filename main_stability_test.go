package main

import (
	"errors"
	"testing"
)

func TestEnabledSlashCommandsPreserveCurrentVisibleSet(t *testing.T) {
	for _, name := range []string{"help", "start", "stop", "link", "unlink", "settings"} {
		if !isSlashCommandEnabled(name) {
			t.Fatalf("expected %q to remain enabled", name)
		}
	}

	for _, name := range []string{
		"refresh",
		"pause",
		"privacy",
		"info",
		"map",
		"stats",
		"premium",
		"debug",
		"download",
		"future-unknown-command",
	} {
		if isSlashCommandEnabled(name) {
			t.Fatalf("expected %q to remain disabled", name)
		}
	}
}

func TestRunMainReturnsZeroOnSuccess(t *testing.T) {
	calls := 0

	got := runMain(func() error {
		calls++
		return nil
	})

	if got != 0 {
		t.Fatalf("runMain() = %d, want 0", got)
	}

	if calls != 1 {
		t.Fatalf("startup function called %d times, want 1", calls)
	}
}

func TestRunMainReturnsOneOnError(t *testing.T) {
	calls := 0

	got := runMain(func() error {
		calls++
		return errors.New("startup failed")
	})

	if got != 1 {
		t.Fatalf("runMain() = %d, want 1", got)
	}

	if calls != 1 {
		t.Fatalf("startup function called %d times, want 1", calls)
	}
}
