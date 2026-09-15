package main

import (
	"testing"

	"github.com/automuteus/automuteus/v8/bot/command"
)

func TestIsSlashCommandEnabledUsesCommandRegistry(t *testing.T) {
	const name = "hostmute"

	original, existed := command.EnabledSlashCommands[name]
	defer func() {
		if existed {
			command.EnabledSlashCommands[name] = original
			return
		}

		delete(command.EnabledSlashCommands, name)
	}()

	command.EnabledSlashCommands[name] = true
	if !isSlashCommandEnabled(name) {
		t.Fatalf("%q should be enabled when command registry enables it", name)
	}

	command.EnabledSlashCommands[name] = false
	if isSlashCommandEnabled(name) {
		t.Fatalf("%q should be disabled when command registry disables it", name)
	}
}

func TestHostMuteEnabledForDiscordRegistration(t *testing.T) {
	if command.HostMute.Name != "hostmute" {
		t.Fatalf("unexpected HostMute command name: %q", command.HostMute.Name)
	}

	if !isSlashCommandEnabled(command.HostMute.Name) {
		t.Fatalf("%q is defined but disabled for Discord registration", command.HostMute.Name)
	}
}
