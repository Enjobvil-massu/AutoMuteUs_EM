package bot

import (
	"strings"
	"testing"
	"unicode"

	"github.com/automuteus/automuteus/v8/pkg/settings"
	"github.com/bwmarrin/discordgo"
)

func containsJapaneseUI(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) {
			return true
		}
	}
	return false
}

func TestManualLinkButtonsContainAll18ColorsAndJapaneseUnlink(t *testing.T) {
	rows := linkColorButtons("starter", "target", "ABC12345")
	colorButtons := 0
	unlinkFound := false
	for _, component := range rows {
		row, ok := component.(discordgo.ActionsRow)
		if !ok {
			t.Fatalf("component was %T, want ActionsRow", component)
		}
		for _, child := range row.Components {
			button, ok := child.(discordgo.Button)
			if !ok {
				t.Fatalf("child was %T, want Button", child)
			}
			if button.Label == "リンク解除" {
				unlinkFound = true
				if button.CustomID != "link-color:starter:target:ABC12345:UNLINK" {
					t.Fatalf("unlink custom ID = %q", button.CustomID)
				}
				continue
			}
			colorButtons++
			if !strings.Contains(button.CustomID, ":ABC12345:") {
				t.Fatalf("manual link button does not contain the game code: %q", button.CustomID)
			}
			if !containsJapaneseUI(button.Label) {
				t.Fatalf("manual link button is not Japanese: %q", button.Label)
			}
		}
	}
	if colorButtons != 18 {
		t.Fatalf("manual link color buttons = %d, want 18", colorButtons)
	}
	if !unlinkFound {
		t.Fatal("Japanese unlink button was not found")
	}
}

func TestPublicUnlinkButtonLabelIsJapanese(t *testing.T) {
	label, useEmoji := buildColorButtonMeta(discordgo.SelectMenuOption{Label: "unlink", Value: UnlinkEmojiName})
	if label != "リンク解除" || useEmoji {
		t.Fatalf("unlink metadata = %q, %v", label, useEmoji)
	}
}

func TestStartControlButtonsAreJapanese(t *testing.T) {
	components := stopButtonComponents("starter", "ABC12345", &settings.GuildSettings{})
	if len(components) != 1 {
		t.Fatalf("start control rows = %d, want 1", len(components))
	}

	row, ok := components[0].(discordgo.ActionsRow)
	if !ok {
		t.Fatalf("component was %T, want ActionsRow", components[0])
	}
	if len(row.Components) != 3 {
		t.Fatalf("start control buttons = %d, want 3", len(row.Components))
	}

	manualLink := row.Components[0].(discordgo.Button)
	refresh := row.Components[1].(discordgo.Button)
	stop := row.Components[2].(discordgo.Button)

	if manualLink.Label != "ホストによる手動リンク" || refresh.Label != "更新" || stop.Label != "停止" {
		t.Fatalf("button labels = %q, %q, %q", manualLink.Label, refresh.Label, stop.Label)
	}
	if manualLink.CustomID != "link-game:starter:ABC12345" {
		t.Fatalf("manual link custom ID = %q", manualLink.CustomID)
	}
	if refresh.CustomID != "refresh-game:starter:ABC12345" {
		t.Fatalf("refresh custom ID = %q", refresh.CustomID)
	}
	if stop.CustomID != "stop-game:starter:ABC12345" {
		t.Fatalf("stop custom ID = %q", stop.CustomID)
	}
}

func TestCaptureWaitingInstructionIsJapanese(t *testing.T) {
	if !containsJapaneseUI(captureWaitingInstruction) {
		t.Fatalf("capture waiting instruction is not Japanese: %q", captureWaitingInstruction)
	}
	for _, forbidden := range []string{"Host URL", "Connection Code", "Click here", "waiting for capture"} {
		if strings.Contains(captureWaitingInstruction, forbidden) {
			t.Fatalf("capture waiting instruction contains English UI text %q: %q", forbidden, captureWaitingInstruction)
		}
	}
}
