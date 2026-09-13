package bot

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestHostTalkAuthorization(t *testing.T) {
	tests := []struct {
		name        string
		leaderID    string
		userID      string
		ownerID     string
		permissions int64
		want        bool
	}{
		{name: "A1 leader", leaderID: "leader", userID: "leader", want: true},
		{name: "A2 guild owner", leaderID: "leader", userID: "owner", ownerID: "owner", want: true},
		{name: "A3 Discord administrator", leaderID: "leader", userID: "admin", permissions: discordgo.PermissionAdministrator | discordgo.PermissionViewChannel, want: true},
		// No EM admin flag, configured user list, or allowed role is an input.
		{name: "A4 management permissions do not imply administrator", userID: "em-admin", permissions: discordgo.PermissionManageServer | discordgo.PermissionManageRoles | discordgo.PermissionVoiceMuteMembers},
		{name: "A5 normal user", leaderID: "leader", userID: "participant", ownerID: "owner", permissions: discordgo.PermissionViewChannel},
		{name: "A6 empty user denied even with administrator bit", permissions: discordgo.PermissionAdministrator},
		{name: "A7 empty leader permits Discord administrator", userID: "admin", permissions: discordgo.PermissionAdministrator, want: true},
		{name: "empty configuration does not authorize everyone", userID: "participant"},
		{name: "empty leader permits guild owner", userID: "owner", ownerID: "owner", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isHostTalkAuthorized(tt.leaderID, tt.userID, tt.ownerID, tt.permissions); got != tt.want {
				t.Fatalf("isHostTalkAuthorized() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHostTalkRevisionTransitions(t *testing.T) {
	tests := []struct {
		name          string
		currentMode   bool
		requestedMode bool
		wantMode      bool
		wantRevision  uint64
		wantChanged   bool
	}{
		{name: "R1 off to on", requestedMode: true, wantMode: true, wantRevision: 8, wantChanged: true},
		{name: "R2 on to off", currentMode: true, wantRevision: 8, wantChanged: true},
		{name: "R3 duplicate on", currentMode: true, requestedMode: true, wantMode: true, wantRevision: 7},
		{name: "R4 duplicate off", wantRevision: 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, revision, changed, err := nextHostTalkState(tt.currentMode, 7, tt.requestedMode)
			if err != nil || mode != tt.wantMode || revision != tt.wantRevision || changed != tt.wantChanged {
				t.Fatalf("nextHostTalkState() = (%v, %d, %v, %v), want (%v, %d, %v, nil)",
					mode, revision, changed, err, tt.wantMode, tt.wantRevision, tt.wantChanged)
			}
		})
	}
}

func TestHostTalkRevisionOnOffOn(t *testing.T) {
	mode, revision := true, uint64(7)
	for _, requested := range []bool{false, true} {
		nextMode, nextRevision, changed, err := nextHostTalkState(mode, revision, requested)
		if err != nil || !changed || nextMode != requested || nextRevision != revision+1 {
			t.Fatalf("transition from (%v, %d) to %v = (%v, %d, %v, %v)",
				mode, revision, requested, nextMode, nextRevision, changed, err)
		}
		mode, revision = nextMode, nextRevision
	}
	if !mode || revision != 9 {
		t.Fatalf("on/off/on ended at (%v, %d), want (true, 9)", mode, revision)
	}
}

func TestHostTalkRevisionOverflow(t *testing.T) {
	const maxRevision = ^uint64(0)
	for _, currentMode := range []bool{false, true} {
		mode, revision, changed, err := nextHostTalkState(currentMode, maxRevision, !currentMode)
		if !errors.Is(err, errHostTalkRevisionOverflow) || changed || mode != currentMode || revision != maxRevision {
			t.Fatalf("overflow changed state: (%v, %d, %v, %v)", mode, revision, changed, err)
		}

		mode, revision, changed, err = nextHostTalkState(currentMode, maxRevision, currentMode)
		if err != nil || changed || mode != currentMode || revision != maxRevision {
			t.Fatalf("duplicate at maximum revision = (%v, %d, %v, %v)", mode, revision, changed, err)
		}
	}
	mode, revision, changed, err := nextHostTalkState(false, maxRevision-1, true)
	if err != nil || !mode || !changed || revision != maxRevision {
		t.Fatalf("last valid increment = (%v, %d, %v, %v)", mode, revision, changed, err)
	}
}

func TestHostTalkVoiceDecision(t *testing.T) {
	tests := []struct {
		name  string
		input hostTalkVoiceInput
		want  hostTalkVoiceDecision
	}{
		{
			name:  "V1 off preserves normal tasks rule",
			input: hostTalkVoiceInput{NormalApplicable: true, NormalMute: true, NormalDeaf: true},
			want:  hostTalkVoiceDecision{Apply: true, Mute: true, Deaf: true},
		},
		{
			name:  "V2 host on",
			input: hostTalkVoiceInput{HostTalkMode: true, LeaderID: "host", UserID: "host", InTrackedVoiceChannel: true, NormalApplicable: true, NormalMute: true, NormalDeaf: true},
			want:  hostTalkVoiceDecision{Apply: true, ManagedAfterSuccess: true},
		},
		{
			name:  "V3 alive participant on",
			input: hostTalkVoiceInput{HostTalkMode: true, LeaderID: "host", UserID: "alive", InTrackedVoiceChannel: true, NormalApplicable: true, NormalMute: true, NormalDeaf: true},
			want:  hostTalkVoiceDecision{Apply: true, Mute: true, ManagedAfterSuccess: true},
		},
		{
			name:  "V4 dead participant on",
			input: hostTalkVoiceInput{HostTalkMode: true, LeaderID: "host", UserID: "dead", InTrackedVoiceChannel: true, NormalApplicable: true},
			want:  hostTalkVoiceDecision{Apply: true, Mute: true, ManagedAfterSuccess: true},
		},
		{
			name:  "V5 unlinked human on",
			input: hostTalkVoiceInput{HostTalkMode: true, LeaderID: "host", UserID: "unlinked", InTrackedVoiceChannel: true, NormalApplicable: false},
			want:  hostTalkVoiceDecision{Apply: true, Mute: true, ManagedAfterSuccess: true},
		},
		{
			name:  "V6 bot excluded even when managed with normal rule",
			input: hostTalkVoiceInput{IsBot: true, HostTalkMode: true, LeaderID: "host", UserID: "bot", InTrackedVoiceChannel: true, WasHostTalkManaged: true, NormalApplicable: true, NormalMute: true, NormalDeaf: true},
			want:  hostTalkVoiceDecision{},
		},
		{
			name:  "V7 outside VC unmanaged without normal rule is untouched",
			input: hostTalkVoiceInput{HostTalkMode: true, LeaderID: "host", UserID: "outside", InTrackedVoiceChannel: false, NormalApplicable: false},
			want:  hostTalkVoiceDecision{},
		},
		{
			name:  "V7 outside VC follows explicitly applicable normal rule",
			input: hostTalkVoiceInput{HostTalkMode: true, LeaderID: "host", UserID: "outside", InTrackedVoiceChannel: false, NormalApplicable: true, NormalDeaf: true},
			want:  hostTalkVoiceDecision{Apply: true, Deaf: true},
		},
		{
			name:  "V8 move out cleans managed human while mode remains on",
			input: hostTalkVoiceInput{HostTalkMode: true, LeaderID: "host", UserID: "moved", InTrackedVoiceChannel: false, WasHostTalkManaged: true, NormalMute: true, NormalDeaf: true},
			want:  hostTalkVoiceDecision{Apply: true},
		},
		{
			name:  "V9 off restores latest normal rule for managed human",
			input: hostTalkVoiceInput{WasHostTalkManaged: true, NormalApplicable: true, NormalMute: true, NormalDeaf: true},
			want:  hostTalkVoiceDecision{Apply: true, Mute: true, Deaf: true},
		},
		{
			name:  "V10 off restores managed unlinked human",
			input: hostTalkVoiceInput{WasHostTalkManaged: true, NormalApplicable: false, NormalMute: true, NormalDeaf: true},
			want:  hostTalkVoiceDecision{Apply: true},
		},
		{
			name:  "V11 missing leader without fallback is untouched",
			input: hostTalkVoiceInput{HostTalkMode: true, UserID: "human", InTrackedVoiceChannel: true},
			want:  hostTalkVoiceDecision{},
		},
		{
			name:  "V11 missing leader uses normal rule",
			input: hostTalkVoiceInput{HostTalkMode: true, UserID: "human", InTrackedVoiceChannel: true, NormalApplicable: true, NormalDeaf: true},
			want:  hostTalkVoiceDecision{Apply: true, Deaf: true},
		},
		{
			name:  "V11 missing leader cleans managed human",
			input: hostTalkVoiceInput{HostTalkMode: true, UserID: "human", InTrackedVoiceChannel: true, WasHostTalkManaged: true},
			want:  hostTalkVoiceDecision{Apply: true},
		},
		{
			name:  "V11 missing leader restores managed human normal rule",
			input: hostTalkVoiceInput{HostTalkMode: true, UserID: "human", InTrackedVoiceChannel: true, WasHostTalkManaged: true, NormalApplicable: true, NormalDeaf: true},
			want:  hostTalkVoiceDecision{Apply: true, Deaf: true},
		},
		{
			name:  "V12 override precedes normal rule and managed cleanup",
			input: hostTalkVoiceInput{HostTalkMode: true, LeaderID: "host", UserID: "human", InTrackedVoiceChannel: true, WasHostTalkManaged: true, NormalApplicable: true, NormalMute: false, NormalDeaf: true},
			want:  hostTalkVoiceDecision{Apply: true, Mute: true, ManagedAfterSuccess: true},
		},
		{
			name:  "V13 unlinked host is managed",
			input: hostTalkVoiceInput{HostTalkMode: true, LeaderID: "host", UserID: "host", InTrackedVoiceChannel: true, NormalApplicable: false},
			want:  hostTalkVoiceDecision{Apply: true, ManagedAfterSuccess: true},
		},
		{
			name:  "off unmanaged without normal rule is untouched",
			input: hostTalkVoiceInput{NormalMute: true, NormalDeaf: true},
			want:  hostTalkVoiceDecision{},
		},
		{
			name:  "bot cleanup remains excluded when mode is off",
			input: hostTalkVoiceInput{IsBot: true, WasHostTalkManaged: true},
			want:  hostTalkVoiceDecision{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decideGameVoiceState(tt.input); got != tt.want {
				t.Fatalf("decideGameVoiceState(%+v) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestHostTalkNormalVoiceRulesPreserved(t *testing.T) {
	for _, mute := range []bool{false, true} {
		for _, deaf := range []bool{false, true} {
			for _, managed := range []bool{false, true} {
				input := hostTalkVoiceInput{
					NormalApplicable:   true,
					NormalMute:         mute,
					NormalDeaf:         deaf,
					WasHostTalkManaged: managed,
				}
				want := hostTalkVoiceDecision{Apply: true, Mute: mute, Deaf: deaf}
				if got := decideGameVoiceState(input); got != want {
					t.Fatalf("normal rule changed: input=%+v got=%+v want=%+v", input, got, want)
				}
			}
		}
	}
}

func TestHostTalkHostManagedThenOffRestoresTasks(t *testing.T) {
	on := decideGameVoiceState(hostTalkVoiceInput{
		HostTalkMode:          true,
		LeaderID:              "host",
		UserID:                "host",
		InTrackedVoiceChannel: true,
		NormalApplicable:      true,
		NormalMute:            true,
		NormalDeaf:            true,
	})
	if !on.Apply || on.Mute || on.Deaf || !on.ManagedAfterSuccess {
		t.Fatalf("host ON decision = %+v", on)
	}
	off := decideGameVoiceState(hostTalkVoiceInput{
		LeaderID:              "host",
		UserID:                "host",
		InTrackedVoiceChannel: true,
		NormalApplicable:      true,
		NormalMute:            true,
		NormalDeaf:            true,
		WasHostTalkManaged:    on.ManagedAfterSuccess,
	})
	if !off.Apply || !off.Mute || !off.Deaf || off.ManagedAfterSuccess {
		t.Fatalf("host OFF decision = %+v", off)
	}
}

func TestHostTalkOldJSONDefaults(t *testing.T) {
	oldJSON := []byte(`{"messageID":"message","messageChannelID":"channel","leaderID":"leader","creationTimeUnix":123}`)
	var got GameStateMessage
	if err := json.Unmarshal(oldJSON, &got); err != nil {
		t.Fatal(err)
	}
	want := GameStateMessage{
		MessageID:        "message",
		MessageChannelID: "channel",
		LeaderID:         "leader",
		CreationTimeUnix: 123,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("old JSON = %+v, want %+v", got, want)
	}
	if got.HostTalkMode || got.HostTalkRevision != 0 || got.HostTalkManagedUsers != nil {
		t.Fatalf("old JSON initialized HostTalk state: %+v", got)
	}
}

func TestHostTalkJSONRoundTrip(t *testing.T) {
	newJSON := []byte(`{"messageID":"message","messageChannelID":"channel","leaderID":"leader","creationTimeUnix":123,"hostTalkMode":true,"hostTalkRevision":7,"hostTalkManagedUsers":{"leader":true,"participant":true}}`)
	want := GameStateMessage{
		MessageID:            "message",
		MessageChannelID:     "channel",
		LeaderID:             "leader",
		CreationTimeUnix:     123,
		HostTalkMode:         true,
		HostTalkRevision:     7,
		HostTalkManagedUsers: map[string]bool{"leader": true, "participant": true},
	}
	var first GameStateMessage
	if err := json.Unmarshal(newJSON, &first); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("new JSON = %+v, want %+v", first, want)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"messageID", "messageChannelID", "leaderID", "creationTimeUnix", "hostTalkMode", "hostTalkRevision", "hostTalkManagedUsers"} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("serialized JSON is missing field %q: %s", name, encoded)
		}
	}
	var roundTrip GameStateMessage
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(roundTrip, want) {
		t.Fatalf("round trip = %+v, want %+v", roundTrip, want)
	}
}

func TestHostTalkZeroStateAndReset(t *testing.T) {
	initial := MakeGameStateMessage()
	if initial.HostTalkMode || initial.HostTalkRevision != 0 || initial.HostTalkManagedUsers != nil {
		t.Fatalf("initial HostTalk state = %+v", initial)
	}
	encoded, err := json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"hostTalkMode", "hostTalkRevision", "hostTalkManagedUsers"} {
		if _, ok := fields[name]; ok {
			t.Fatalf("zero-value optional field %q was serialized", name)
		}
	}

	dgs := NewDiscordGameState("guild")
	dgs.GameStateMsg = GameStateMessage{
		LeaderID:             "leader",
		HostTalkMode:         true,
		HostTalkRevision:     7,
		HostTalkManagedUsers: map[string]bool{"leader": true},
	}
	dgs.Reset()
	if !reflect.DeepEqual(dgs.GameStateMsg, initial) || dgs.GuildID != "guild" {
		t.Fatalf("reset state = %+v, guild = %q", dgs.GameStateMsg, dgs.GuildID)
	}
}

func TestPlanHostTalkGameStateRejectsNil(t *testing.T) {
	candidate, changed, err := planHostTalkGameState(nil, true)

	if !errors.Is(err, errHostTalkGameStateUnavailable) {
		t.Fatalf("err = %v, want errHostTalkGameStateUnavailable", err)
	}
	if candidate != nil {
		t.Fatalf("candidate = %#v, want nil", candidate)
	}
	if changed {
		t.Fatal("nil GameState reported a state change")
	}
}

func TestPlanHostTalkGameStateOffToOnDoesNotMutateOriginal(t *testing.T) {
	current := NewDiscordGameState("guild-1")
	current.ConnectCode = "ABCDEFGH"
	current.VoiceChannel = "voice-1"
	current.GameStateMsg.LeaderID = "host-1"
	current.GameStateMsg.HostTalkMode = false
	current.GameStateMsg.HostTalkRevision = 7
	current.GameStateMsg.HostTalkManagedUsers = map[string]bool{
		"user-1": true,
	}

	candidate, changed, err := planHostTalkGameState(current, true)
	if err != nil {
		t.Fatalf("planHostTalkGameState() error = %v", err)
	}
	if !changed {
		t.Fatal("OFF -> ON did not report a change")
	}

	if current.GameStateMsg.HostTalkMode {
		t.Fatal("original GameState HostTalkMode was mutated")
	}
	if current.GameStateMsg.HostTalkRevision != 7 {
		t.Fatalf("original revision = %d, want 7", current.GameStateMsg.HostTalkRevision)
	}

	if !candidate.GameStateMsg.HostTalkMode {
		t.Fatal("candidate HostTalkMode = false, want true")
	}
	if candidate.GameStateMsg.HostTalkRevision != 8 {
		t.Fatalf("candidate revision = %d, want 8", candidate.GameStateMsg.HostTalkRevision)
	}

	if candidate.GuildID != current.GuildID ||
		candidate.ConnectCode != current.ConnectCode ||
		candidate.VoiceChannel != current.VoiceChannel ||
		candidate.GameStateMsg.LeaderID != current.GameStateMsg.LeaderID {
		t.Fatalf("non-HostTalk game identity changed: candidate=%+v current=%+v", candidate, current)
	}

	if !reflect.DeepEqual(
		candidate.GameStateMsg.HostTalkManagedUsers,
		current.GameStateMsg.HostTalkManagedUsers,
	) {
		t.Fatalf(
			"managed users changed: candidate=%v current=%v",
			candidate.GameStateMsg.HostTalkManagedUsers,
			current.GameStateMsg.HostTalkManagedUsers,
		)
	}
}

func TestPlanHostTalkGameStateOnToOffPreservesManagedUsers(t *testing.T) {
	current := NewDiscordGameState("guild-1")
	current.GameStateMsg.HostTalkMode = true
	current.GameStateMsg.HostTalkRevision = 11
	current.GameStateMsg.HostTalkManagedUsers = map[string]bool{
		"host-1": true,
		"user-1": true,
		"user-2": true,
	}

	candidate, changed, err := planHostTalkGameState(current, false)
	if err != nil {
		t.Fatalf("planHostTalkGameState() error = %v", err)
	}
	if !changed {
		t.Fatal("ON -> OFF did not report a change")
	}
	if candidate.GameStateMsg.HostTalkMode {
		t.Fatal("candidate HostTalkMode = true, want false")
	}
	if candidate.GameStateMsg.HostTalkRevision != 12 {
		t.Fatalf("candidate revision = %d, want 12", candidate.GameStateMsg.HostTalkRevision)
	}
	if !reflect.DeepEqual(
		candidate.GameStateMsg.HostTalkManagedUsers,
		current.GameStateMsg.HostTalkManagedUsers,
	) {
		t.Fatalf(
			"managed users were cleared or modified: candidate=%v current=%v",
			candidate.GameStateMsg.HostTalkManagedUsers,
			current.GameStateMsg.HostTalkManagedUsers,
		)
	}
}

func TestPlanHostTalkGameStateDuplicateRequestIsIdempotent(t *testing.T) {
	for _, mode := range []bool{false, true} {
		t.Run(map[bool]string{false: "duplicate OFF", true: "duplicate ON"}[mode], func(t *testing.T) {
			current := NewDiscordGameState("guild-1")
			current.GameStateMsg.HostTalkMode = mode
			current.GameStateMsg.HostTalkRevision = 21
			current.GameStateMsg.HostTalkManagedUsers = map[string]bool{
				"user-1": true,
			}

			candidate, changed, err := planHostTalkGameState(current, mode)
			if err != nil {
				t.Fatalf("planHostTalkGameState() error = %v", err)
			}
			if changed {
				t.Fatal("duplicate explicit mode request reported a change")
			}
			if candidate.GameStateMsg.HostTalkMode != mode {
				t.Fatalf("candidate mode = %v, want %v", candidate.GameStateMsg.HostTalkMode, mode)
			}
			if candidate.GameStateMsg.HostTalkRevision != 21 {
				t.Fatalf("candidate revision = %d, want 21", candidate.GameStateMsg.HostTalkRevision)
			}
			if !reflect.DeepEqual(
				candidate.GameStateMsg.HostTalkManagedUsers,
				current.GameStateMsg.HostTalkManagedUsers,
			) {
				t.Fatal("duplicate request changed managed users")
			}
		})
	}
}

func TestPlanHostTalkGameStateRevisionOverflowPreservesState(t *testing.T) {
	current := NewDiscordGameState("guild-1")
	current.GameStateMsg.HostTalkMode = false
	current.GameStateMsg.HostTalkRevision = ^uint64(0)
	current.GameStateMsg.HostTalkManagedUsers = map[string]bool{
		"user-1": true,
	}

	candidate, changed, err := planHostTalkGameState(current, true)

	if !errors.Is(err, errHostTalkRevisionOverflow) {
		t.Fatalf("err = %v, want errHostTalkRevisionOverflow", err)
	}
	if changed {
		t.Fatal("overflow transition reported a state change")
	}

	if candidate.GameStateMsg.HostTalkMode != current.GameStateMsg.HostTalkMode {
		t.Fatal("overflow changed HostTalkMode")
	}
	if candidate.GameStateMsg.HostTalkRevision != current.GameStateMsg.HostTalkRevision {
		t.Fatal("overflow changed HostTalkRevision")
	}
	if !reflect.DeepEqual(
		candidate.GameStateMsg.HostTalkManagedUsers,
		current.GameStateMsg.HostTalkManagedUsers,
	) {
		t.Fatal("overflow changed managed users")
	}
}
