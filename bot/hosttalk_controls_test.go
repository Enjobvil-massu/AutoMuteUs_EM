package bot

import (
	"errors"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestHostTalkControlIDRoundTrip(t *testing.T) {
	tests := []struct {
		name          string
		requestedMode bool
		revision      uint64
	}{
		{
			name:          "on",
			requestedMode: true,
			revision:      7,
		},
		{
			name:          "off",
			requestedMode: false,
			revision:      8,
		},
		{
			name:          "maximum revision",
			requestedMode: true,
			revision:      ^uint64(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := buildHostTalkControlID(
				"ABCDEF",
				1700000000,
				tt.revision,
				tt.requestedMode,
			)
			if err != nil {
				t.Fatal(err)
			}

			if len(id) > 100 {
				t.Fatalf("custom ID length = %d, want <= 100", len(id))
			}

			got, ok := parseHostTalkControlID(id)
			if !ok {
				t.Fatalf("failed to parse generated custom ID %q", id)
			}

			if got.ConnectCode != "ABCDEF" ||
				got.GameCreationTimeUnix != 1700000000 ||
				got.ExpectedRevision != tt.revision ||
				got.RequestedMode != tt.requestedMode {
				t.Fatalf("round trip = %#v", got)
			}
		})
	}
}

func TestHostTalkControlIDRejectsInvalid(t *testing.T) {
	invalidBuilds := []struct {
		code     string
		creation int64
	}{
		{
			code:     "",
			creation: 1700000000,
		},
		{
			code:     "ABC:DEF",
			creation: 1700000000,
		},
		{
			code:     "ABCDEF",
			creation: 0,
		},
		{
			code:     strings.Repeat("A", 90),
			creation: 1700000000,
		},
	}

	for _, tt := range invalidBuilds {
		if _, err := buildHostTalkControlID(
			tt.code,
			tt.creation,
			1,
			true,
		); !errors.Is(err, errHostTalkControlIDInvalid) {
			t.Fatalf(
				"buildHostTalkControlID(%q, %d) error = %v",
				tt.code,
				tt.creation,
				err,
			)
		}
	}

	invalidIDs := []string{
		"",
		"hosttalk-mode",
		"other:ABCDEF:1700000000:1:on",
		"hosttalk-mode::1700000000:1:on",
		"hosttalk-mode:ABCDEF:bad:1:on",
		"hosttalk-mode:ABCDEF:0:1:on",
		"hosttalk-mode:ABCDEF:1700000000:bad:on",
		"hosttalk-mode:ABCDEF:1700000000:1:maybe",
		"hosttalk-mode:ABCDEF:1700000000:1:on:extra",
	}

	for _, id := range invalidIDs {
		if got, ok := parseHostTalkControlID(id); ok {
			t.Fatalf("invalid ID %q parsed as %#v", id, got)
		}
	}
}

func TestHostTalkControlMatchesGame(t *testing.T) {
	dgs := NewDiscordGameState("guild")
	dgs.ConnectCode = "ABCDEF"
	dgs.GameStateMsg.CreationTimeUnix = 1700000000
	dgs.GameStateMsg.HostTalkRevision = 9

	current := hostTalkControl{
		ConnectCode:          "abcdef",
		GameCreationTimeUnix: 1700000000,
		ExpectedRevision:     9,
		RequestedMode:        true,
	}

	if !hostTalkControlMatchesGame(dgs, current) {
		t.Fatal("current control was rejected")
	}

	staleRevision := current
	staleRevision.ExpectedRevision = 8
	if hostTalkControlMatchesGame(dgs, staleRevision) {
		t.Fatal("stale revision was accepted")
	}

	staleGame := current
	staleGame.GameCreationTimeUnix--
	if hostTalkControlMatchesGame(dgs, staleGame) {
		t.Fatal("old game generation was accepted")
	}

	wrongCode := current
	wrongCode.ConnectCode = "ZZZZZZ"
	if hostTalkControlMatchesGame(dgs, wrongCode) {
		t.Fatal("wrong connect code was accepted")
	}

	if hostTalkControlMatchesGame(nil, current) {
		t.Fatal("nil game was accepted")
	}
}

func TestHostTalkSelectorContent(t *testing.T) {
	on := hostTalkSelectorContent(true)
	if !strings.Contains(on, "🎙️ ホスト発言モード") ||
		!strings.Contains(on, "**ON**") {
		t.Fatalf("unexpected ON selector content: %q", on)
	}

	off := hostTalkSelectorContent(false)
	if !strings.Contains(off, "🎙️ ホスト発言モード") ||
		!strings.Contains(off, "**OFF**") {
		t.Fatalf("unexpected OFF selector content: %q", off)
	}
}

func TestHostTalkSelectorComponents(t *testing.T) {
	components, err := hostTalkSelectorComponents(
		"ABCDEF",
		1700000000,
		11,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(components) != 1 {
		t.Fatalf("row count = %d, want 1", len(components))
	}

	row, ok := components[0].(discordgo.ActionsRow)
	if !ok {
		t.Fatalf("component type = %T, want ActionsRow", components[0])
	}

	if len(row.Components) != 2 {
		t.Fatalf("button count = %d, want 2", len(row.Components))
	}

	onButton, ok := row.Components[0].(discordgo.Button)
	if !ok {
		t.Fatalf("ON component type = %T", row.Components[0])
	}

	offButton, ok := row.Components[1].(discordgo.Button)
	if !ok {
		t.Fatalf("OFF component type = %T", row.Components[1])
	}

	if onButton.Label != "ONにする" {
		t.Fatalf("ON label = %q", onButton.Label)
	}

	if offButton.Label != "OFFにする" {
		t.Fatalf("OFF label = %q", offButton.Label)
	}

	onControl, ok := parseHostTalkControlID(onButton.CustomID)
	if !ok || !onControl.RequestedMode {
		t.Fatalf("invalid ON control: %#v, parsed=%v", onControl, ok)
	}

	offControl, ok := parseHostTalkControlID(offButton.CustomID)
	if !ok || offControl.RequestedMode {
		t.Fatalf("invalid OFF control: %#v, parsed=%v", offControl, ok)
	}

	if onControl.ExpectedRevision != 11 ||
		offControl.ExpectedRevision != 11 {
		t.Fatal("selector buttons do not share expected revision")
	}

	if onControl.GameCreationTimeUnix != 1700000000 ||
		offControl.GameCreationTimeUnix != 1700000000 {
		t.Fatal("selector buttons do not share game generation")
	}
}

func TestHostTalkEnablePreflight(t *testing.T) {
	guild := &discordgo.Guild{
		ID: "guild",
		VoiceStates: []*discordgo.VoiceState{
			{
				UserID:    "host",
				ChannelID: "tracked",
			},
			{
				UserID:    "participant",
				ChannelID: "tracked",
			},
		},
	}

	if !hostTalkEnablePreflight(
		guild,
		"host",
		"tracked",
	) {
		t.Fatal("host in tracked VC was rejected")
	}

	if hostTalkEnablePreflight(
		guild,
		"host",
		"other",
	) {
		t.Fatal("host in another VC was accepted")
	}

	if hostTalkEnablePreflight(
		guild,
		"missing",
		"tracked",
	) {
		t.Fatal("host outside voice was accepted")
	}

	if hostTalkEnablePreflight(
		nil,
		"host",
		"tracked",
	) {
		t.Fatal("nil guild was accepted")
	}
}
