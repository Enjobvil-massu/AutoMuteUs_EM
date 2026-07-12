package bot

import (
	"testing"
	"time"

	"github.com/automuteus/automuteus/v8/pkg/amongus"
	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
)

func TestChooseDiscordDisplayNamePriority(t *testing.T) {
	tests := []struct {
		name       string
		nick       string
		globalName string
		username   string
		userID     string
		want       string
	}{
		{name: "server nickname", nick: "サーバー表示名", globalName: "Discord表示名", username: "account_name", userID: "1", want: "サーバー表示名"},
		{name: "global display name", globalName: "Discord表示名", username: "account_name", userID: "1", want: "Discord表示名"},
		{name: "account username", username: "account_name", userID: "1", want: "account_name"},
		{name: "user id fallback", userID: "1", want: "1"},
		{name: "unknown fallback", want: "不明なユーザー"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chooseDiscordDisplayName(tt.nick, tt.globalName, tt.username, tt.userID)
			if got != tt.want {
				t.Fatalf("chooseDiscordDisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEndGameChannelSignalIsNonBlockingAndSingleUse(t *testing.T) {
	bot := &Bot{EndGameChannels: make(map[string]chan EndGameMessage)}
	endGameChannel := make(chan EndGameMessage, 1)
	bot.registerEndGameChannel("ABCDEFGH", endGameChannel)

	done := make(chan bool, 1)
	go func() {
		done <- bot.signalEndGame("ABCDEFGH")
	}()

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("signalEndGame() returned false")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("signalEndGame() blocked")
	}

	select {
	case signal := <-endGameChannel:
		if !signal {
			t.Fatal("end-game signal was false")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("end-game signal was not delivered")
	}

	if bot.signalEndGame("ABCDEFGH") {
		t.Fatal("second signalEndGame() should not find an already removed channel")
	}
}

func TestReplacingEndGameWorkerSignalsOldWorker(t *testing.T) {
	bot := &Bot{EndGameChannels: make(map[string]chan EndGameMessage)}
	oldChannel := make(chan EndGameMessage, 1)
	newChannel := make(chan EndGameMessage, 1)

	bot.registerEndGameChannel("ABCDEFGH", oldChannel)
	bot.registerEndGameChannel("ABCDEFGH", newChannel)

	select {
	case <-oldChannel:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("old subscriber worker was not signaled")
	}
}

func TestResetCaptureTimerDrainsOldTick(t *testing.T) {
	timer := time.NewTimer(10 * time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	resetCaptureTimer(timer, 50*time.Millisecond)

	select {
	case <-timer.C:
		t.Fatal("an old timer tick leaked after reset")
	case <-time.After(20 * time.Millisecond):
	}

	select {
	case <-timer.C:
	case <-time.After(80 * time.Millisecond):
		t.Fatal("reset timer did not fire")
	}
}

func TestSignalAllEndGamesSignalsAndClearsWorkers(t *testing.T) {
	bot := &Bot{EndGameChannels: make(map[string]chan EndGameMessage)}
	first := make(chan EndGameMessage, 1)
	second := make(chan EndGameMessage, 1)
	bot.registerEndGameChannel("FIRST", first)
	bot.registerEndGameChannel("SECOND", second)

	if got := bot.signalAllEndGames(); got != 2 {
		t.Fatalf("signalAllEndGames() = %d, want 2", got)
	}

	for name, ch := range map[string]chan EndGameMessage{"first": first, "second": second} {
		select {
		case signal := <-ch:
			if !signal {
				t.Fatalf("%s worker received false signal", name)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("%s worker did not receive shutdown signal", name)
		}
	}

	bot.ChannelsMapLock.RLock()
	remaining := len(bot.EndGameChannels)
	bot.ChannelsMapLock.RUnlock()
	if remaining != 0 {
		t.Fatalf("%d end-game worker(s) remained registered", remaining)
	}
}

func TestIsUserInTrackedVoiceChannel(t *testing.T) {
	guild := &discordgo.Guild{VoiceStates: []*discordgo.VoiceState{
		{UserID: "user-1", ChannelID: "voice-a"},
		{UserID: "user-2", ChannelID: "voice-b"},
	}}

	if !isUserInTrackedVoiceChannel(guild, "user-1", "voice-a") {
		t.Fatal("tracked voice participant was rejected")
	}
	if isUserInTrackedVoiceChannel(guild, "user-1", "voice-b") {
		t.Fatal("participant in another voice channel was accepted")
	}
	if isUserInTrackedVoiceChannel(nil, "user-1", "voice-a") {
		t.Fatal("nil guild was accepted")
	}
}

func TestIsCurrentSelfLinkPanel(t *testing.T) {
	dgs := NewDiscordGameState("guild-1")
	dgs.GameStateMsg = GameStateMessage{
		MessageID:        "message-1",
		MessageChannelID: "text-1",
	}

	if !isCurrentSelfLinkPanel(dgs, "message-1", "text-1") {
		t.Fatal("current self-link panel was rejected")
	}
	if isCurrentSelfLinkPanel(dgs, "old-message", "text-1") {
		t.Fatal("old self-link panel was accepted")
	}
	if isCurrentSelfLinkPanel(dgs, "message-1", "other-channel") {
		t.Fatal("panel from another channel was accepted")
	}
}

func TestIsCurrentStartControlGame(t *testing.T) {
	dgs := NewDiscordGameState("guild-1")
	dgs.ConnectCode = "ABC12345"

	if !isCurrentStartControlGame(dgs, "ABC12345") {
		t.Fatal("current /start control was rejected")
	}
	if !isCurrentStartControlGame(dgs, "abc12345") {
		t.Fatal("current /start control with different letter case was rejected")
	}
	if isCurrentStartControlGame(dgs, "OLD12345") {
		t.Fatal("stale /start control was accepted")
	}
	if isCurrentStartControlGame(dgs, "") {
		t.Fatal("empty game code was accepted")
	}
	if isCurrentStartControlGame(nil, "ABC12345") {
		t.Fatal("nil game state was accepted")
	}
}

func TestLinkOrUnlinkRejectsNilGameState(t *testing.T) {
	bot := &Bot{}
	resp, success := bot.linkOrUnlinkAndRespond(nil, "user-1", "Red", settings.MakeGuildSettings())
	if success {
		t.Fatal("nil game state reported a successful link")
	}
	if resp == nil || resp.Data == nil || resp.Data.Content == "" {
		t.Fatal("nil game state did not return a user-facing response")
	}
}

func TestLinkedPlayerNameForUser(t *testing.T) {
	dgs := NewDiscordGameState("guild-1")
	dgs.UserData["linked"] = UserData{InGameName: "Player One"}
	dgs.UserData["unlinked"] = UserData{InGameName: amongus.UnlinkedPlayerName}

	if got, ok := linkedPlayerNameForUser(dgs, "linked"); !ok || got != "Player One" {
		t.Fatalf("linked player = %q, %v; want Player One, true", got, ok)
	}
	if _, ok := linkedPlayerNameForUser(dgs, "unlinked"); ok {
		t.Fatal("unlinked user was reported as linked")
	}
}

func TestLinkedUserForPlayerName(t *testing.T) {
	dgs := NewDiscordGameState("guild-1")
	dgs.UserData["user-1"] = UserData{InGameName: "Player One"}
	dgs.UserData["user-2"] = UserData{InGameName: amongus.UnlinkedPlayerName}

	if got, ok := linkedUserForPlayerName(dgs, "Player One", "user-3"); !ok || got != "user-1" {
		t.Fatalf("linked user = %q, %v; want user-1, true", got, ok)
	}
	if _, ok := linkedUserForPlayerName(dgs, "Player One", "user-1"); ok {
		t.Fatal("excepted user was treated as a duplicate")
	}
	if _, ok := linkedUserForPlayerName(dgs, amongus.UnlinkedPlayerName, ""); ok {
		t.Fatal("unlinked sentinel was treated as a duplicate")
	}
}
