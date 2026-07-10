package command

import (
	"strings"
	"testing"

	"github.com/automuteus/automuteus/v8/pkg/settings"
)

func TestNewResponseAddsLaunchLinkAndPreservesCopyCodeBlocks(t *testing.T) {
	resp := NewResponse(NewSuccess, NewInfo{
		ApiHyperlink: "https://capture.example.com/open/link?connectCode=ABCDEFGH",
		MinimalURL:   "https://amu.example.com:443",
		ConnectCode:  "ABCDEFGH",
	}, &settings.GuildSettings{})

	if resp == nil || resp.Data == nil {
		t.Fatal("NewResponse returned nil response data")
	}
	if !strings.Contains(resp.Data.Content, "https://capture.example.com/open/link?connectCode=ABCDEFGH") {
		t.Fatalf("launch link was not added: %q", resp.Data.Content)
	}
	if len(resp.Data.Embeds) != 1 || len(resp.Data.Embeds[0].Fields) < 2 {
		t.Fatal("host/code embed fields were not preserved")
	}
	if got := resp.Data.Embeds[0].Fields[0].Value; got != "```https://amu.example.com```" {
		t.Fatalf("host code block changed: %q", got)
	}
	if got := resp.Data.Embeds[0].Fields[1].Value; !strings.HasPrefix(got, "```ABCDEFGH```\n") {
		t.Fatalf("connect-code block changed: %q", got)
	}
}

func TestNewResponseHidesLaunchLineWhenURLIsUnavailable(t *testing.T) {
	resp := NewResponse(NewSuccess, NewInfo{
		MinimalURL:  "https://amu.example.com:443",
		ConnectCode: "ABCDEFGH",
	}, &settings.GuildSettings{})

	if resp == nil || resp.Data == nil {
		t.Fatal("NewResponse returned nil response data")
	}
	if resp.Data.Content != "" {
		t.Fatalf("launch content should be empty when API URL is unavailable: %q", resp.Data.Content)
	}
}
