package main

import "testing"

func TestEnabledSlashCommandsPreserveCurrentVisibleSet(t *testing.T) {
	for _, name := range []string{"help", "start", "stop", "link", "unlink", "settings"} {
		if !isSlashCommandEnabled(name) {
			t.Fatalf("expected %q to remain enabled", name)
		}
	}

	for _, name := range []string{"refresh", "pause", "privacy", "info", "map", "stats", "premium", "debug", "download", "future-unknown-command"} {
		if isSlashCommandEnabled(name) {
			t.Fatalf("expected %q to remain disabled", name)
		}
	}
}
