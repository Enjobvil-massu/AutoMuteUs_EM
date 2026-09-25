package bot

import (
	"strings"
	"testing"
)

func TestMakeBotBuildsWithoutStartingGateway(t *testing.T) {
	b := MakeBot(
		"test-version",
		"test-commit",
		"test-token",
		"",
		"http://localhost:8123",
		"",
		1,
		0,
		nil,
		nil,
		nil,
		"",
	)
	if b == nil {
		t.Fatal("MakeBot returned nil")
	}
	if b.PrimarySession == nil {
		t.Fatal("MakeBot did not create a Discord session")
	}
	if b.TokenProvider != nil {
		t.Fatal("MakeBot unexpectedly installed a TokenProvider")
	}
}

func TestStartRefusesWithoutTokenProvider(t *testing.T) {
	err := (&Bot{}).Start()
	if err == nil {
		t.Fatal("Start succeeded without a TokenProvider")
	}
	if !strings.Contains(err.Error(), "TokenProvider") {
		t.Fatalf("Start error = %q, want TokenProvider guard", err)
	}
}
