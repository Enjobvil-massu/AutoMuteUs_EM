package bot

import (
	"reflect"
	"testing"
)

func TestQuiesceVoiceResetState(t *testing.T) {
	dgs := NewDiscordGameState("guild")
	dgs.ConnectCode = "connect"
	dgs.VoiceChannel = "voice"
	dgs.Running = true
	dgs.GameStateMsg.HostTalkMode = true
	dgs.GameStateMsg.HostTalkRevision = 77
	dgs.GameStateMsg.HostTalkManagedUsers = map[string]bool{
		"111": true,
		"222": false,
	}

	managedBefore := map[string]bool{}
	for userID, managed := range dgs.GameStateMsg.HostTalkManagedUsers {
		managedBefore[userID] = managed
	}

	if changed := quiesceVoiceResetState(dgs); !changed {
		t.Fatal("running game was not reported as quiesced")
	}

	if dgs.Running {
		t.Fatal("Running remained true after quiesce")
	}

	if dgs.GuildID != "guild" {
		t.Fatalf("GuildID changed: %q", dgs.GuildID)
	}

	if dgs.ConnectCode != "connect" {
		t.Fatalf("ConnectCode changed: %q", dgs.ConnectCode)
	}

	if dgs.VoiceChannel != "voice" {
		t.Fatalf("VoiceChannel changed: %q", dgs.VoiceChannel)
	}

	if !dgs.GameStateMsg.HostTalkMode {
		t.Fatal("HostTalkMode changed during quiesce")
	}

	if dgs.GameStateMsg.HostTalkRevision != 77 {
		t.Fatalf(
			"HostTalkRevision changed: %d",
			dgs.GameStateMsg.HostTalkRevision,
		)
	}

	if !reflect.DeepEqual(
		dgs.GameStateMsg.HostTalkManagedUsers,
		managedBefore,
	) {
		t.Fatalf(
			"HostTalkManagedUsers changed: got %#v, want %#v",
			dgs.GameStateMsg.HostTalkManagedUsers,
			managedBefore,
		)
	}

	if changed := quiesceVoiceResetState(dgs); changed {
		t.Fatal("already-quiesced game was reported as newly changed")
	}
}

func TestQuiesceVoiceResetStateNil(t *testing.T) {
	if quiesceVoiceResetState(nil) {
		t.Fatal("nil game state was reported as changed")
	}
}
