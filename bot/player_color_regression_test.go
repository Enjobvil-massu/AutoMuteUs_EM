package bot

import (
	"testing"

	"github.com/automuteus/automuteus/v8/pkg/amongus"
	"github.com/automuteus/automuteus/v8/pkg/settings"
)

func TestToEmojiEmbedFieldsSkipsInvalidColorAndKeepsFollowingPlayers(t *testing.T) {
	dgs := NewDiscordGameState("guild-1")
	dgs.UserData["user-1"] = UserData{
		User: User{
			UserID:   "user-1",
			UserName: "tester",
		},
		InGameName: "Valid Player",
	}
	dgs.DisplayNames["user-1"] = "表示名"

	emojis := emptyStatusEmojis()
	for alive, list := range emojis {
		for i := range list {
			emojis[alive][i] = Emoji{Name: "AliveRed", ID: "1"}
		}
	}

	players := []amongus.PlayerData{
		{Color: 18, Name: "Invalid Player", IsAlive: true},
		{Color: 0, Name: "Valid Player", IsAlive: true},
	}

	fields := dgs.toEmojiEmbedFieldsForPlayers(players, emojis, settings.MakeGuildSettings())
	if len(fields) != 1 {
		t.Fatalf("field count = %d, want 1", len(fields))
	}
	if fields[0].Name != "Valid Player（表示名）" {
		t.Fatalf("field name = %q, want valid player after invalid color", fields[0].Name)
	}
}
