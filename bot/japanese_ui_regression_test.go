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
	rows := linkColorButtons("starter", "target")
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
				continue
			}
			colorButtons++
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
	components := stopButtonComponents("starter", &settings.GuildSettings{})
	row := components[0].(discordgo.ActionsRow)
	first := row.Components[0].(discordgo.Button)
	second := row.Components[1].(discordgo.Button)
	if first.Label != "手動リンク" || second.Label != "停止" {
		t.Fatalf("button labels = %q, %q", first.Label, second.Label)
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
