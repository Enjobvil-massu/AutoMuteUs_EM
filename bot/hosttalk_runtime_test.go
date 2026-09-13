package bot

import (
	"errors"
	"reflect"
	"testing"

	"github.com/automuteus/automuteus/v8/pkg/game"
)

func TestRefreshHostTalkVoiceMembersUsesCurrentObservation(t *testing.T) {
	members := []hostTalkVoiceMember{
		{
			UserID:                "host",
			InTrackedVoiceChannel: false,
			NormalApplicable:      true,
			NormalMute:            true,
			NormalDeaf:            true,
		},
		{
			UserID:                "participant",
			InTrackedVoiceChannel: false,
			NormalApplicable:      true,
			NormalMute:            false,
			NormalDeaf:            true,
		},
		{
			UserID:                "bot",
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
		},
		{
			UserID: "unknown",
		},
	}

	before := append([]hostTalkVoiceMember(nil), members...)

	observations := map[string]hostTalkVoiceObservation{
		"host": {
			UserID:                "host",
			InTrackedVoiceChannel: true,
		},
		"participant": {
			UserID:                "participant",
			InTrackedVoiceChannel: true,
		},
		"bot": {
			UserID:                "bot",
			IsBot:                 true,
			InTrackedVoiceChannel: true,
		},
	}

	got := refreshHostTalkVoiceMembers(members, observations)

	want := []hostTalkVoiceMember{
		{
			UserID:                "host",
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
			NormalMute:            true,
			NormalDeaf:            true,
		},
		{
			UserID:                "participant",
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
			NormalMute:            false,
			NormalDeaf:            true,
		},
		{
			UserID:                "bot",
			IsBot:                 true,
			InTrackedVoiceChannel: true,
			NormalApplicable:      true,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("refreshed members = %#v, want %#v", got, want)
	}

	if !reflect.DeepEqual(members, before) {
		t.Fatalf("input members mutated: %#v", members)
	}
}

func TestRefreshHostTalkVoiceMembersAllowsUncachedTrackedHumanOverride(t *testing.T) {
	members := []hostTalkVoiceMember{
		{
			UserID:                "uncached-human",
			InTrackedVoiceChannel: false,
			NormalApplicable:      false,
		},
	}

	observations := map[string]hostTalkVoiceObservation{
		"uncached-human": {
			UserID:                "uncached-human",
			IsBot:                 false,
			InTrackedVoiceChannel: true,
		},
	}

	refreshed := refreshHostTalkVoiceMembers(members, observations)
	plans := planHostTalkVoiceBatch(true, "host", refreshed)

	want := []hostTalkVoicePlan{
		{
			UserID:              "uncached-human",
			Mute:                true,
			Deaf:                false,
			ManagedAfterSuccess: true,
		},
	}

	if !reflect.DeepEqual(plans, want) {
		t.Fatalf("uncached tracked human plans = %#v, want %#v", plans, want)
	}
}

func TestRefreshHostTalkVoiceMembersExcludesResolvedBotFromPlan(t *testing.T) {
	members := []hostTalkVoiceMember{
		{
			UserID:                "bot",
			InTrackedVoiceChannel: true,
		},
	}

	observations := map[string]hostTalkVoiceObservation{
		"bot": {
			UserID:                "bot",
			IsBot:                 true,
			InTrackedVoiceChannel: true,
		},
	}

	refreshed := refreshHostTalkVoiceMembers(members, observations)
	plans := planHostTalkVoiceBatch(true, "host", refreshed)

	if len(plans) != 0 {
		t.Fatalf("resolved bot produced HostTalk plan: %#v", plans)
	}
}

func TestRecordHostTalkSuccessfulVoiceRecordsPersistsManagedState(t *testing.T) {
	bot, redisInterface, _, gsr := newHostTalkRedisTestBot(t)

	state := redisInterface.getDiscordGameState(gsr, false)
	if state == nil {
		t.Fatal("seeded state missing")
	}

	state.GameData.Phase = game.TASKS
	state.GameStateMsg.HostTalkMode = true
	state.GameStateMsg.HostTalkRevision = 9
	state.UserData = UserDataSet{
		"host-1": {
			User: User{
				UserID: "host-1",
			},
			ShouldBeMute: true,
			ShouldBeDeaf: true,
		},
		"participant": {
			User: User{
				UserID: "participant",
			},
			ShouldBeMute: false,
			ShouldBeDeaf: true,
		},
	}

	redisInterface.SetDiscordGameState(state, nil)

	err := bot.recordHostTalkSuccessfulVoiceRecords(
		gsr,
		game.TASKS,
		true,
		9,
		[]hostTalkVoiceRecord{
			{
				UserID:                        "host-1",
				Mute:                          false,
				Deaf:                          false,
				ManagedAfterSuccess:           true,
				ExpectedInTrackedVoiceChannel: true,
			},
			{
				UserID:                        "participant",
				Mute:                          true,
				Deaf:                          false,
				ManagedAfterSuccess:           true,
				ExpectedInTrackedVoiceChannel: true,
			},
			{
				UserID:                        "uncached-human",
				Mute:                          true,
				Deaf:                          false,
				ManagedAfterSuccess:           true,
				ExpectedInTrackedVoiceChannel: true,
			},
		},
	)
	if err != nil {
		t.Fatalf("recordHostTalkSuccessfulVoiceRecords() error = %v", err)
	}

	stored := redisInterface.getDiscordGameState(gsr, false)
	if stored == nil {
		t.Fatal("stored state missing after HostTalk successful record")
	}

	host := stored.UserData["host-1"]
	if host.ShouldBeMute || host.ShouldBeDeaf {
		t.Fatalf("host ShouldBe = mute:%v deaf:%v, want false/false", host.ShouldBeMute, host.ShouldBeDeaf)
	}

	participant := stored.UserData["participant"]
	if !participant.ShouldBeMute || participant.ShouldBeDeaf {
		t.Fatalf(
			"participant ShouldBe = mute:%v deaf:%v, want true/false",
			participant.ShouldBeMute,
			participant.ShouldBeDeaf,
		)
	}

	for _, userID := range []string{"host-1", "participant", "uncached-human"} {
		if !stored.GameStateMsg.HostTalkManagedUsers[userID] {
			t.Fatalf("%s was not persisted as HostTalk-managed", userID)
		}
	}
}

func TestRecordHostTalkSuccessfulVoiceRecordsRejectsStaleRevision(t *testing.T) {
	bot, redisInterface, _, gsr := newHostTalkRedisTestBot(t)

	state := redisInterface.getDiscordGameState(gsr, false)
	if state == nil {
		t.Fatal("seeded state missing")
	}

	state.GameData.Phase = game.TASKS
	state.GameStateMsg.HostTalkMode = true
	state.GameStateMsg.HostTalkRevision = 12
	state.UserData = UserDataSet{
		"participant": {
			User: User{
				UserID: "participant",
			},
		},
	}

	redisInterface.SetDiscordGameState(state, nil)

	err := bot.recordHostTalkSuccessfulVoiceRecords(
		gsr,
		game.TASKS,
		true,
		11,
		[]hostTalkVoiceRecord{
			{
				UserID:              "participant",
				Mute:                true,
				ManagedAfterSuccess: true,
			},
		},
	)

	if !errors.Is(err, errHostTalkVoiceSnapshotStale) {
		t.Fatalf("error = %v, want errHostTalkVoiceSnapshotStale", err)
	}

	stored := redisInterface.getDiscordGameState(gsr, false)
	if stored == nil {
		t.Fatal("stored state missing after stale record attempt")
	}
	if stored.UserData["participant"].ShouldBeMute {
		t.Fatal("stale record changed ShouldBeMute")
	}
	if stored.GameStateMsg.HostTalkManagedUsers["participant"] {
		t.Fatal("stale record added managed user")
	}
}
