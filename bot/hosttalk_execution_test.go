package bot

import (
	"errors"
	"reflect"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func hostTalkExecutionTestGuild() *discordgo.Guild {
	return &discordgo.Guild{
		ID: "guild-1",
		Members: []*discordgo.Member{
			{
				User: &discordgo.User{
					ID:  "host",
					Bot: false,
				},
			},
			{
				User: &discordgo.User{
					ID:  "participant",
					Bot: false,
				},
			},
			{
				User: &discordgo.User{
					ID:  "outside-human",
					Bot: false,
				},
			},
			{
				User: &discordgo.User{
					ID:  "music-bot",
					Bot: true,
				},
			},
		},
		VoiceStates: []*discordgo.VoiceState{
			{
				UserID:    "host",
				ChannelID: "tracked",
				Mute:      false,
				Deaf:      false,
			},
			{
				UserID:    "participant",
				ChannelID: "tracked",
				Mute:      false,
				Deaf:      false,
			},
			{
				UserID:    "outside-human",
				ChannelID: "other",
				Mute:      true,
				Deaf:      false,
			},
			{
				UserID:    "music-bot",
				ChannelID: "tracked",
				Mute:      false,
				Deaf:      false,
			},
			{
				UserID:    "unknown-member",
				ChannelID: "tracked",
				Mute:      false,
				Deaf:      false,
			},
		},
	}
}

func TestObserveHostTalkGuildVoiceMembersFailClosedForUnknownMember(t *testing.T) {
	guild := hostTalkExecutionTestGuild()

	got := observeHostTalkGuildVoiceMembers(guild, "tracked")

	if len(got) != 4 {
		t.Fatalf("observation count = %d, want 4", len(got))
	}
	if _, ok := got["unknown-member"]; ok {
		t.Fatal("unknown member was not filtered fail-closed")
	}

	host := got["host"]
	if host.IsBot {
		t.Fatal("host incorrectly classified as bot")
	}
	if !host.InTrackedVoiceChannel {
		t.Fatal("host should be in tracked voice channel")
	}
	if host.Mute || host.Deaf {
		t.Fatalf("host actual state = mute:%v deaf:%v, want false/false", host.Mute, host.Deaf)
	}

	outside := got["outside-human"]
	if outside.InTrackedVoiceChannel {
		t.Fatal("outside human incorrectly classified as tracked")
	}
	if !outside.Mute || outside.Deaf {
		t.Fatalf("outside actual state = mute:%v deaf:%v, want true/false", outside.Mute, outside.Deaf)
	}

	if !got["music-bot"].IsBot {
		t.Fatal("Discord bot was not identified")
	}
}

func TestObserveHostTalkGuildVoiceMembersNilGuild(t *testing.T) {
	got := observeHostTalkGuildVoiceMembers(nil, "tracked")

	if got == nil {
		t.Fatal("nil guild should return deterministic empty map")
	}
	if len(got) != 0 {
		t.Fatalf("nil guild observation count = %d, want 0", len(got))
	}
}

func TestPartitionHostTalkVoicePlansUsesActualDiscordState(t *testing.T) {
	observations := observeHostTalkGuildVoiceMembers(
		hostTalkExecutionTestGuild(),
		"tracked",
	)

	plans := []hostTalkVoicePlan{
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
			UserID:              "outside-human",
			Mute:                false,
			Deaf:                false,
			ManagedAfterSuccess: false,
		},
		{
			UserID:              "music-bot",
			Mute:                true,
			Deaf:                false,
			ManagedAfterSuccess: true,
		},
		{
			UserID:              "unknown-member",
			Mute:                true,
			Deaf:                false,
			ManagedAfterSuccess: true,
		},
	}

	already, pending := partitionHostTalkVoicePlans(plans, observations)

	wantAlready := []hostTalkVoiceRecord{
		{
			UserID:                        "host",
			Mute:                          false,
			Deaf:                          false,
			ManagedAfterSuccess:           true,
			ExpectedInTrackedVoiceChannel: true,
		},
	}

	wantPending := []hostTalkPendingVoicePlan{
		{
			Plan: hostTalkVoicePlan{
				UserID:              "participant",
				Mute:                true,
				Deaf:                false,
				ManagedAfterSuccess: true,
			},
			ExpectedInTrackedVoiceChannel: true,
		},
		{
			Plan: hostTalkVoicePlan{
				UserID:              "outside-human",
				Mute:                false,
				Deaf:                false,
				ManagedAfterSuccess: false,
			},
			ExpectedInTrackedVoiceChannel: false,
		},
	}

	if !reflect.DeepEqual(already, wantAlready) {
		t.Fatalf("already = %#v, want %#v", already, wantAlready)
	}
	if !reflect.DeepEqual(pending, wantPending) {
		t.Fatalf("pending = %#v, want %#v", pending, wantPending)
	}
}

func TestFilterHostTalkVoiceRecordsRevalidatesVoiceBotAndActualState(t *testing.T) {
	records := []hostTalkVoiceRecord{
		{
			UserID:                        "host",
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
			UserID:                        "outside-human",
			Mute:                          true,
			Deaf:                          false,
			ManagedAfterSuccess:           false,
			ExpectedInTrackedVoiceChannel: false,
		},
		{
			UserID:                        "music-bot",
			Mute:                          false,
			Deaf:                          false,
			ManagedAfterSuccess:           true,
			ExpectedInTrackedVoiceChannel: true,
		},
		{
			UserID:                        "missing",
			Mute:                          false,
			Deaf:                          false,
			ManagedAfterSuccess:           true,
			ExpectedInTrackedVoiceChannel: true,
		},
	}

	observations := observeHostTalkGuildVoiceMembers(
		hostTalkExecutionTestGuild(),
		"tracked",
	)

	got := filterHostTalkVoiceRecords(
		records,
		observations,
		true,
	)

	want := []hostTalkVoiceRecord{
		{
			UserID:                        "host",
			Mute:                          false,
			Deaf:                          false,
			ManagedAfterSuccess:           true,
			ExpectedInTrackedVoiceChannel: true,
		},
		{
			UserID:                        "outside-human",
			Mute:                          true,
			Deaf:                          false,
			ManagedAfterSuccess:           false,
			ExpectedInTrackedVoiceChannel: false,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered records = %#v, want %#v", got, want)
	}
}

func TestFilterHostTalkVoiceRecordsRejectsTrackedChannelMove(t *testing.T) {
	records := []hostTalkVoiceRecord{
		{
			UserID:                        "participant",
			Mute:                          true,
			Deaf:                          false,
			ManagedAfterSuccess:           true,
			ExpectedInTrackedVoiceChannel: true,
		},
	}

	observations := map[string]hostTalkVoiceObservation{
		"participant": {
			UserID:                "participant",
			InTrackedVoiceChannel: false,
			Mute:                  true,
			Deaf:                  false,
		},
	}

	got := filterHostTalkVoiceRecords(
		records,
		observations,
		true,
	)

	if len(got) != 0 {
		t.Fatalf("moved member was retained: %#v", got)
	}
}

func TestFilterHostTalkPendingVoicePlansIsLastPreSendGate(t *testing.T) {
	pending := []hostTalkPendingVoicePlan{
		{
			Plan: hostTalkVoicePlan{
				UserID:              "participant",
				Mute:                true,
				Deaf:                false,
				ManagedAfterSuccess: true,
			},
			ExpectedInTrackedVoiceChannel: true,
		},
		{
			Plan: hostTalkVoicePlan{
				UserID:              "outside-human",
				Mute:                false,
				Deaf:                false,
				ManagedAfterSuccess: false,
			},
			ExpectedInTrackedVoiceChannel: false,
		},
		{
			Plan: hostTalkVoicePlan{
				UserID:              "music-bot",
				Mute:                true,
				Deaf:                false,
				ManagedAfterSuccess: true,
			},
			ExpectedInTrackedVoiceChannel: true,
		},
	}

	observations := observeHostTalkGuildVoiceMembers(
		hostTalkExecutionTestGuild(),
		"tracked",
	)

	got := filterHostTalkPendingVoicePlans(
		pending,
		observations,
	)

	want := pending[:2]

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pre-send filtered = %#v, want %#v", got, want)
	}
}

func TestFilterHostTalkPendingVoicePlansRejectsMoveBeforeSend(t *testing.T) {
	pending := []hostTalkPendingVoicePlan{
		{
			Plan: hostTalkVoicePlan{
				UserID:              "participant",
				Mute:                true,
				Deaf:                false,
				ManagedAfterSuccess: true,
			},
			ExpectedInTrackedVoiceChannel: true,
		},
	}

	observations := map[string]hostTalkVoiceObservation{
		"participant": {
			UserID:                "participant",
			InTrackedVoiceChannel: false,
		},
	}

	got := filterHostTalkPendingVoicePlans(
		pending,
		observations,
	)

	if len(got) != 0 {
		t.Fatalf("moved member remained in pre-send list: %#v", got)
	}
}

func TestRecordsFromSuccessfulHostTalkPendingRequiresSameVoiceSideButNotImmediateActualEcho(t *testing.T) {
	pending := []hostTalkPendingVoicePlan{
		{
			Plan: hostTalkVoicePlan{
				UserID:              "participant",
				Mute:                true,
				Deaf:                false,
				ManagedAfterSuccess: true,
			},
			ExpectedInTrackedVoiceChannel: true,
		},
	}

	// Discord's state event may not have echoed the successful server mute yet.
	// Membership/bot status must still match, but immediate actual mute equality
	// is deliberately not required after ModifyUsers reports success.
	observations := map[string]hostTalkVoiceObservation{
		"participant": {
			UserID:                "participant",
			InTrackedVoiceChannel: true,
			Mute:                  false,
			Deaf:                  false,
		},
	}

	got := recordsFromSuccessfulHostTalkPending(
		pending,
		observations,
	)

	want := []hostTalkVoiceRecord{
		{
			UserID:                        "participant",
			Mute:                          true,
			Deaf:                          false,
			ManagedAfterSuccess:           true,
			ExpectedInTrackedVoiceChannel: true,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("successful records = %#v, want %#v", got, want)
	}
}

func TestRecordsFromSuccessfulHostTalkPendingRejectsMoveAfterSend(t *testing.T) {
	pending := []hostTalkPendingVoicePlan{
		{
			Plan: hostTalkVoicePlan{
				UserID:              "participant",
				Mute:                true,
				Deaf:                false,
				ManagedAfterSuccess: true,
			},
			ExpectedInTrackedVoiceChannel: true,
		},
	}

	observations := map[string]hostTalkVoiceObservation{
		"participant": {
			UserID:                "participant",
			InTrackedVoiceChannel: false,
			Mute:                  true,
			Deaf:                  false,
		},
	}

	got := recordsFromSuccessfulHostTalkPending(
		pending,
		observations,
	)

	if len(got) != 0 {
		t.Fatalf("post-send moved member was recorded: %#v", got)
	}
}

func TestExecutionSnapshotHelpersDoNotMutateInputs(t *testing.T) {
	plans := []hostTalkVoicePlan{
		{
			UserID:              "participant",
			Mute:                true,
			Deaf:                false,
			ManagedAfterSuccess: true,
		},
	}

	observations := map[string]hostTalkVoiceObservation{
		"participant": {
			UserID:                "participant",
			InTrackedVoiceChannel: true,
			Mute:                  false,
			Deaf:                  false,
		},
	}

	plansBefore := append([]hostTalkVoicePlan(nil), plans...)
	observationsBefore := map[string]hostTalkVoiceObservation{
		"participant": observations["participant"],
	}

	_, pending := partitionHostTalkVoicePlans(
		plans,
		observations,
	)
	_ = filterHostTalkPendingVoicePlans(
		pending,
		observations,
	)
	_ = recordsFromSuccessfulHostTalkPending(
		pending,
		observations,
	)

	if !reflect.DeepEqual(plans, plansBefore) {
		t.Fatalf("plans mutated: %#v", plans)
	}
	if !reflect.DeepEqual(observations, observationsBefore) {
		t.Fatalf("observations mutated: %#v", observations)
	}
}

func TestObserveHostTalkGuildVoiceMembersWithResolverIncludesUnknownHuman(t *testing.T) {
	guild := hostTalkExecutionTestGuild()
	calls := 0

	got := observeHostTalkGuildVoiceMembersWithResolver(
		guild,
		"tracked",
		func(userID string) (*discordgo.Member, error) {
			calls++
			if userID != "unknown-member" {
				t.Fatalf("resolver called for cached user %q", userID)
			}
			return &discordgo.Member{
				User: &discordgo.User{
					ID:  userID,
					Bot: false,
				},
			}, nil
		},
	)

	if calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", calls)
	}

	observation, ok := got["unknown-member"]
	if !ok {
		t.Fatal("resolved unknown human was not included")
	}
	if observation.IsBot {
		t.Fatal("resolved human incorrectly classified as bot")
	}
	if !observation.InTrackedVoiceChannel {
		t.Fatal("resolved human should be in tracked VC")
	}
}

func TestObserveHostTalkGuildVoiceMembersWithResolverClassifiesResolvedBot(t *testing.T) {
	guild := hostTalkExecutionTestGuild()

	got := observeHostTalkGuildVoiceMembersWithResolver(
		guild,
		"tracked",
		func(userID string) (*discordgo.Member, error) {
			return &discordgo.Member{
				User: &discordgo.User{
					ID:  userID,
					Bot: true,
				},
			}, nil
		},
	)

	observation, ok := got["unknown-member"]
	if !ok {
		t.Fatal("resolved bot observation missing")
	}
	if !observation.IsBot {
		t.Fatal("resolved bot was not identified as bot")
	}
}

func TestObserveHostTalkGuildVoiceMembersWithResolverFailureRemainsFailClosed(t *testing.T) {
	guild := hostTalkExecutionTestGuild()

	got := observeHostTalkGuildVoiceMembersWithResolver(
		guild,
		"tracked",
		func(string) (*discordgo.Member, error) {
			return nil, errors.New("simulated member lookup failure")
		},
	)

	if _, ok := got["unknown-member"]; ok {
		t.Fatal("resolver failure should remain fail-closed")
	}
}

func TestObserveHostTalkGuildVoiceMembersWithResolverRejectsWrongIdentity(t *testing.T) {
	guild := hostTalkExecutionTestGuild()

	got := observeHostTalkGuildVoiceMembersWithResolver(
		guild,
		"tracked",
		func(string) (*discordgo.Member, error) {
			return &discordgo.Member{
				User: &discordgo.User{
					ID:  "different-user",
					Bot: false,
				},
			}, nil
		},
	)

	if _, ok := got["unknown-member"]; ok {
		t.Fatal("resolver result for wrong Discord identity was accepted")
	}
}

func TestObserveHostTalkGuildVoiceMembersWithResolverDoesNotResolveCachedMembers(t *testing.T) {
	guild := hostTalkExecutionTestGuild()
	guild.VoiceStates = guild.VoiceStates[:4]

	calls := 0

	_ = observeHostTalkGuildVoiceMembersWithResolver(
		guild,
		"tracked",
		func(string) (*discordgo.Member, error) {
			calls++
			return nil, errors.New("resolver should not be called")
		},
	)

	if calls != 0 {
		t.Fatalf("resolver called %d times for cached members", calls)
	}
}
