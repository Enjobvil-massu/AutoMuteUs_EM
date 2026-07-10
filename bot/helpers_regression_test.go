package bot

import (
	"strings"
	"testing"
)

func TestFormCaptureURLKeepsManualConnectionWithoutPublicAPIURL(t *testing.T) {
	t.Setenv("API_SERVER_URL", "")

	hyperlink, apiHyperlink, minimalURL := formCaptureURL("https://amu.example.com", "ABCDEFGH")
	if hyperlink != "aucapture://amu.example.com:443/ABCDEFGH" {
		t.Fatalf("unexpected direct Capture URL: %q", hyperlink)
	}
	if apiHyperlink != "" {
		t.Fatalf("API launch link should be hidden without API_SERVER_URL, got %q", apiHyperlink)
	}
	if minimalURL != "https://amu.example.com:443" {
		t.Fatalf("unexpected manual host: %q", minimalURL)
	}
}

func TestFormCaptureURLBuildsPublicLaunchLink(t *testing.T) {
	t.Setenv("API_SERVER_URL", "https://capture.example.com/")

	_, apiHyperlink, _ := formCaptureURL("https://amu.example.com", "ABCDEFGH")
	want := "https://capture.example.com/open/link?connectCode=ABCDEFGH"
	if apiHyperlink != want {
		t.Fatalf("apiHyperlink = %q, want %q", apiHyperlink, want)
	}
}

func TestFormCaptureURLRejectsNonHTTPAPIURL(t *testing.T) {
	t.Setenv("API_SERVER_URL", "javascript:alert(1)")

	_, apiHyperlink, _ := formCaptureURL("https://amu.example.com", "ABCDEFGH")
	if strings.TrimSpace(apiHyperlink) != "" {
		t.Fatalf("unsafe API launch link was not rejected: %q", apiHyperlink)
	}
}
