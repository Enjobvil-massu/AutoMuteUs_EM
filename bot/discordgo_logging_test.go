package bot

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestShouldDropDiscordgoMessage(t *testing.T) {
	unknown := "unknown event: Op: %d, Seq: %d, Type: %s, Data: %s"
	cases := []struct {
		name   string
		level  int
		format string
		drop   bool
	}{
		{"unknown event warning", discordgo.LogWarning, unknown, true},
		{"other warning", discordgo.LogWarning, "error unmarshalling %s event, %s", false},
		{"error level", discordgo.LogError, "error closing websocket, %s", false},
		{"bookkeeping called", discordgo.LogInformational, "called", true},
		{"bookkeeping exiting", discordgo.LogInformational, "exiting", true},
		{"bookkeeping hello", discordgo.LogInformational, "Op 10 Hello Packet received from Discord", true},
		{"exact match only", discordgo.LogInformational, "called %s", false},
		{"connecting retained", discordgo.LogInformational, "connecting to gateway %s", false},
		{"resume retained", discordgo.LogInformational, "sending resume packet to gateway", false},
		{"reconnecting retained", discordgo.LogInformational, "trying to reconnect to gateway", false},
		{"reconnected retained", discordgo.LogInformational, "successfully reconnected to gateway", false},
		{"rate limit retained", discordgo.LogInformational, "Rate Limiting %s, retry in %v", false},
		{"502 retry retained", discordgo.LogInformational, "%s Failed (%s), Retrying...", false},
		{"unknown event text at error level", discordgo.LogError, unknown, false},
	}
	for _, c := range cases {
		if got := shouldDropDiscordgoMessage(c.level, c.format); got != c.drop {
			t.Errorf("%s: drop = %v, want %v", c.name, got, c.drop)
		}
	}
}

func TestDiscordgoLoggerOutput(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	discordgoLogger(discordgo.LogWarning, 0, "unknown event: Op: %d, Type: %s", 0, "VOICE_CHANNEL_START_TIME_UPDATE")
	if buf.Len() != 0 {
		t.Fatalf("unknown event warning should be dropped, got %q", buf.String())
	}

	discordgoLogger(discordgo.LogInformational, 0, "called")
	if buf.Len() != 0 {
		t.Fatalf("bookkeeping message should be dropped, got %q", buf.String())
	}

	discordgoLogger(discordgo.LogWarning, 0, "something %s", "bad")
	out := buf.String()
	if !strings.HasPrefix(out[strings.Index(out, "["):], "[DG1] ") {
		t.Errorf("missing discordgo level prefix: %q", out)
	}
	if !strings.Contains(out, "something bad") {
		t.Errorf("message not passed through: %q", out)
	}
	if !strings.Contains(out, "discordgo_logging_test.go") {
		t.Errorf("caller should resolve to the message source, got %q", out)
	}
}
