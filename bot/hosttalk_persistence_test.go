package bot

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/automuteus/automuteus/v8/pkg/rediskey"
	"github.com/bsm/redislock"
	"github.com/go-redis/redis/v8"
)

func newHostTalkRedisTestBot(t *testing.T) (*Bot, *RedisInterface, *miniredis.Miniredis, GameStateRequest) {
	t.Helper()

	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run() error = %v", err)
	}

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		server.Close()
	})

	redisInterface := &RedisInterface{client: client}
	bot := &Bot{RedisInterface: redisInterface}

	dgs := NewDiscordGameState("guild-1")
	dgs.ConnectCode = "ABCDEFGH"
	dgs.VoiceChannel = "voice-1"
	dgs.GameStateMsg.MessageChannelID = "text-1"
	dgs.GameStateMsg.LeaderID = "host-1"
	dgs.Running = true

	redisInterface.SetDiscordGameState(dgs, nil)

	gsr := GameStateRequest{
		GuildID:     "guild-1",
		ConnectCode: "ABCDEFGH",
	}

	if stored := redisInterface.getDiscordGameState(gsr, false); stored == nil {
		t.Fatal("failed to seed HostTalk Redis test GameState")
	}

	return bot, redisInterface, server, gsr
}

func TestCommitHostTalkGameStatePersistenceFailurePreservesOriginal(t *testing.T) {
	current := NewDiscordGameState("guild-1")
	current.GameStateMsg.HostTalkMode = false
	current.GameStateMsg.HostTalkRevision = 3

	persistErr := errors.New("simulated persistence failure")
	calls := 0

	result, changed, err := commitHostTalkGameState(
		current,
		true,
		func(candidate *GameState) error {
			calls++
			if !candidate.GameStateMsg.HostTalkMode {
				t.Fatal("persistence candidate was not ON")
			}
			if candidate.GameStateMsg.HostTalkRevision != 4 {
				t.Fatalf("candidate revision = %d, want 4", candidate.GameStateMsg.HostTalkRevision)
			}
			return persistErr
		},
	)

	if !errors.Is(err, errHostTalkPersistFailed) {
		t.Fatalf("err = %v, want errHostTalkPersistFailed", err)
	}
	if !errors.Is(err, persistErr) {
		t.Fatalf("err = %v, want wrapped persistence error", err)
	}
	if changed {
		t.Fatal("failed persistence reported a committed change")
	}
	if result != current {
		t.Fatal("failed persistence did not return the original state")
	}
	if calls != 1 {
		t.Fatalf("persistence calls = %d, want 1", calls)
	}
	if current.GameStateMsg.HostTalkMode {
		t.Fatal("failed persistence mutated original HostTalkMode")
	}
	if current.GameStateMsg.HostTalkRevision != 3 {
		t.Fatalf("original revision = %d, want 3", current.GameStateMsg.HostTalkRevision)
	}
}

func TestCommitHostTalkGameStateDuplicateSkipsPersistence(t *testing.T) {
	current := NewDiscordGameState("guild-1")
	current.GameStateMsg.HostTalkMode = true
	current.GameStateMsg.HostTalkRevision = 9

	calls := 0
	result, changed, err := commitHostTalkGameState(
		current,
		true,
		func(*GameState) error {
			calls++
			return nil
		},
	)

	if err != nil {
		t.Fatalf("commitHostTalkGameState() error = %v", err)
	}
	if changed {
		t.Fatal("duplicate ON reported a committed change")
	}
	if result != current {
		t.Fatal("duplicate ON did not return original state")
	}
	if calls != 0 {
		t.Fatalf("duplicate ON persistence calls = %d, want 0", calls)
	}
	if current.GameStateMsg.HostTalkRevision != 9 {
		t.Fatalf("revision = %d, want 9", current.GameStateMsg.HostTalkRevision)
	}
}

func TestCommitHostTalkGameStateSuccessCommitsCandidate(t *testing.T) {
	current := NewDiscordGameState("guild-1")
	current.GameStateMsg.HostTalkRevision = 12

	calls := 0
	result, changed, err := commitHostTalkGameState(
		current,
		true,
		func(candidate *GameState) error {
			calls++
			if candidate == current {
				t.Fatal("persistence candidate reused original pointer")
			}
			return nil
		},
	)

	if err != nil {
		t.Fatalf("commitHostTalkGameState() error = %v", err)
	}
	if !changed {
		t.Fatal("OFF -> ON did not commit")
	}
	if result == current {
		t.Fatal("successful commit returned original state pointer")
	}
	if calls != 1 {
		t.Fatalf("persistence calls = %d, want 1", calls)
	}
	if !result.GameStateMsg.HostTalkMode {
		t.Fatal("committed state is not ON")
	}
	if result.GameStateMsg.HostTalkRevision != 13 {
		t.Fatalf("committed revision = %d, want 13", result.GameStateMsg.HostTalkRevision)
	}
	if current.GameStateMsg.HostTalkMode {
		t.Fatal("successful commit mutated original state")
	}
	if current.GameStateMsg.HostTalkRevision != 12 {
		t.Fatalf("original revision = %d, want 12", current.GameStateMsg.HostTalkRevision)
	}
}

func TestSetHostTalkModePersistsAndDuplicateIsIdempotent(t *testing.T) {
	bot, redisInterface, _, gsr := newHostTalkRedisTestBot(t)

	updated, changed, err := bot.setHostTalkMode(gsr, true)
	if err != nil {
		t.Fatalf("setHostTalkMode(ON) error = %v", err)
	}
	if !changed {
		t.Fatal("initial HostTalk ON did not report a change")
	}
	if !updated.GameStateMsg.HostTalkMode {
		t.Fatal("returned HostTalk state is not ON")
	}
	if updated.GameStateMsg.HostTalkRevision != 1 {
		t.Fatalf("returned revision = %d, want 1", updated.GameStateMsg.HostTalkRevision)
	}

	stored := redisInterface.getDiscordGameState(gsr, false)
	if stored == nil {
		t.Fatal("stored GameState disappeared after HostTalk ON")
	}
	if !stored.GameStateMsg.HostTalkMode {
		t.Fatal("stored HostTalk state is not ON")
	}
	if stored.GameStateMsg.HostTalkRevision != 1 {
		t.Fatalf("stored revision = %d, want 1", stored.GameStateMsg.HostTalkRevision)
	}

	dataKey := rediskey.ConnectCodeData(gsr.GuildID, gsr.ConnectCode)
	if got := redisInterface.CheckPointer(rediskey.ConnectCodePtr(gsr.GuildID, gsr.ConnectCode)); got != dataKey {
		t.Fatalf("connect-code pointer = %q, want %q", got, dataKey)
	}

	again, changed, err := bot.setHostTalkMode(gsr, true)
	if err != nil {
		t.Fatalf("duplicate setHostTalkMode(ON) error = %v", err)
	}
	if changed {
		t.Fatal("duplicate HostTalk ON reported another change")
	}
	if again.GameStateMsg.HostTalkRevision != 1 {
		t.Fatalf("duplicate ON revision = %d, want 1", again.GameStateMsg.HostTalkRevision)
	}

	stored = redisInterface.getDiscordGameState(gsr, false)
	if stored.GameStateMsg.HostTalkRevision != 1 {
		t.Fatalf("stored duplicate ON revision = %d, want 1", stored.GameStateMsg.HostTalkRevision)
	}
}

func TestSetHostTalkModeMissingGameDoesNotCreateState(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run() error = %v", err)
	}
	defer server.Close()

	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()

	bot := &Bot{
		RedisInterface: &RedisInterface{client: client},
	}

	gsr := GameStateRequest{
		GuildID:     "guild-missing",
		ConnectCode: "MISSING1",
	}

	result, changed, err := bot.setHostTalkMode(gsr, true)

	if !errors.Is(err, errHostTalkGameStateUnavailable) {
		t.Fatalf("err = %v, want errHostTalkGameStateUnavailable", err)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	if changed {
		t.Fatal("missing game reported a committed change")
	}
	if len(server.Keys()) != 0 {
		t.Fatalf("missing game created Redis keys: %v", server.Keys())
	}
}

func TestSetHostTalkModeLockContentionDoesNotMutate(t *testing.T) {
	bot, redisInterface, _, gsr := newHostTalkRedisTestBot(t)

	lockKey := rediskey.ConnectCodeData(gsr.GuildID, gsr.ConnectCode) + ":lock"

	held, err := redislock.New(redisInterface.client).Obtain(
		ctx,
		lockKey,
		3*time.Second,
		nil,
	)
	if err != nil {
		t.Fatalf("failed to hold game-state lock: %v", err)
	}
	defer func() {
		_ = held.Release(ctx)
	}()

	result, changed, err := bot.setHostTalkMode(gsr, true)

	if !errors.Is(err, errHostTalkLockUnavailable) {
		t.Fatalf("err = %v, want errHostTalkLockUnavailable", err)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil while lock unavailable", result)
	}
	if changed {
		t.Fatal("lock contention reported a committed change")
	}

	stored := redisInterface.getDiscordGameState(gsr, false)
	if stored == nil {
		t.Fatal("stored state disappeared during lock contention")
	}
	if stored.GameStateMsg.HostTalkMode {
		t.Fatal("lock contention changed stored HostTalkMode")
	}
	if stored.GameStateMsg.HostTalkRevision != 0 {
		t.Fatalf("lock contention changed revision to %d", stored.GameStateMsg.HostTalkRevision)
	}
}

func TestSetHostTalkModeConcurrentExplicitOnIsSerialized(t *testing.T) {
	bot, redisInterface, _, gsr := newHostTalkRedisTestBot(t)

	type result struct {
		changed bool
		err     error
	}

	start := make(chan struct{})
	results := make(chan result, 2)

	var wg sync.WaitGroup
	wg.Add(2)

	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			<-start

			_, changed, err := bot.setHostTalkMode(gsr, true)
			results <- result{
				changed: changed,
				err:     err,
			}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	changedCount := 0

	for item := range results {
		if item.err != nil {
			t.Fatalf("concurrent HostTalk ON error = %v", item.err)
		}
		if item.changed {
			changedCount++
		}
	}

	if changedCount != 1 {
		t.Fatalf("committed concurrent ON count = %d, want 1", changedCount)
	}

	stored := redisInterface.getDiscordGameState(gsr, false)
	if stored == nil {
		t.Fatal("stored state disappeared after concurrent HostTalk ON")
	}
	if !stored.GameStateMsg.HostTalkMode {
		t.Fatal("final concurrent HostTalk mode is not ON")
	}
	if stored.GameStateMsg.HostTalkRevision != 1 {
		t.Fatalf("final concurrent revision = %d, want 1", stored.GameStateMsg.HostTalkRevision)
	}
}
