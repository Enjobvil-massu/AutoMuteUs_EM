package command

import (
	"strings"
	"testing"

	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
)

func TestNewResponseShowsCompactLaunchAndCodeBlocks(t *testing.T) {
	resp := NewResponse(NewSuccess, NewInfo{
		ApiHyperlink: "https://capture.example.com/open/link?connectCode=ABCDEFGH",
		MinimalURL:   "https://amu.example.com:443",
		ConnectCode:  "ABCDEFGH",
	}, &settings.GuildSettings{})

	if resp == nil || resp.Data == nil {
		t.Fatal("NewResponse returned nil response data")
	}
	if len(resp.Data.Embeds) != 0 {
		t.Fatalf("host/code must not be placed in embeds: %d embed(s)", len(resp.Data.Embeds))
	}

	content := resp.Data.Content
	checks := []string{
		"https://capture.example.com/open/link?connectCode=ABCDEFGH",
		"```text\nhttps://amu.example.com\n```",
		"```text\nABCDEFGH\n```",
		"AutoMuteUsを開始しました",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Fatalf("response content does not contain %q: %q", want, content)
		}
	}

	if strings.Contains(content, "ダウンロード") || strings.Contains(content, "releases/latest") {
		t.Fatalf("download link must not be included in the compact response: %q", content)
	}
	if resp.Data.Flags&discordgo.MessageFlagsEphemeral == 0 {
		t.Fatal("successful /start response must remain ephemeral")
	}
	if len(content) > 1000 {
		t.Fatalf("compact /start response is unexpectedly long: %d characters", len(content))
	}
}

func TestNewResponseKeepsManualConnectionWhenLaunchURLIsUnavailable(t *testing.T) {
	resp := NewResponse(NewSuccess, NewInfo{
		MinimalURL:  "https://amu.example.com:443",
		ConnectCode: "ABCDEFGH",
	}, &settings.GuildSettings{})

	if resp == nil || resp.Data == nil {
		t.Fatal("NewResponse returned nil response data")
	}

	content := resp.Data.Content
	checks := []string{
		"現在利用できません",
		"```text\nhttps://amu.example.com\n```",
		"```text\nABCDEFGH\n```",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Fatalf("manual connection information is missing %q: %q", want, content)
		}
	}
}

func TestNewResponseRejectsUnsafeLaunchURL(t *testing.T) {
	resp := NewResponse(NewSuccess, NewInfo{
		ApiHyperlink: "javascript:alert(1)",
		MinimalURL:   "https://amu.example.com:443",
		ConnectCode:  "ABCDEFGH",
	}, &settings.GuildSettings{})

	if resp == nil || resp.Data == nil {
		t.Fatal("NewResponse returned nil response data")
	}
	if strings.Contains(resp.Data.Content, "javascript:") {
		t.Fatalf("unsafe launch URL was included: %q", resp.Data.Content)
	}
}
