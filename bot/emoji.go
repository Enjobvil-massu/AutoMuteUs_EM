package bot

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/automuteus/automuteus/v8/pkg/game"

	"github.com/bwmarrin/discordgo"
)

const (
	UnlinkEmojiName = "auunlink"
	X               = "❌"
	ThumbsUp        = "👍"
	Hourglass       = "⌛"
)

// Emoji struct for discord
type Emoji struct {
	Name string
	ID   string
}

// FormatForInline does what it sounds like
func (e *Emoji) FormatForInline() string {
	return "<:" + e.Name + ":" + e.ID + ">"
}

// GetDiscordCDNUrl does what it sounds like
func (e *Emoji) GetDiscordCDNUrl() string {
	return "https://cdn.discordapp.com/emojis/" + e.ID + ".png"
}

const maxEmojiDownloadBytes int64 = 2 * 1024 * 1024

var emojiHTTPClient = &http.Client{Timeout: 15 * time.Second}

// DownloadAndBase64Encode downloads an emoji image without allowing a network
// error, non-success response, or unexpectedly large body to crash the bot.
func (e *Emoji) DownloadAndBase64Encode() (string, error) {
	return downloadAndBase64Encode(emojiHTTPClient, e.GetDiscordCDNUrl())
}

func downloadAndBase64Encode(client *http.Client, url string) (string, error) {
	if client == nil {
		return "", errors.New("emoji HTTP client is nil")
	}

	response, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download emoji from %s: %w", url, err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("download emoji from %s: unexpected HTTP status %s", url, response.Status)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, maxEmojiDownloadBytes+1))
	if err != nil {
		return "", fmt.Errorf("read emoji from %s: %w", url, err)
	}
	if int64(len(data)) > maxEmojiDownloadBytes {
		return "", fmt.Errorf("download emoji from %s: response exceeds %d bytes", url, maxEmojiDownloadBytes)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("download emoji from %s: empty response body", url)
	}

	encodedStr := base64.StdEncoding.EncodeToString(data)
	return "data:image/png;base64," + encodedStr, nil
}

func (a AlivenessEmojis) isEmpty() bool {
	if v, ok := a[true]; ok {
		for _, vv := range v {
			if vv.Name == "" || vv.ID == "" {
				return true
			}
		}
	} else {
		return true
	}
	if v, ok := a[false]; ok {
		for _, vv := range v {
			if vv.Name == "" || vv.ID == "" {
				return true
			}
		}
	} else {
		return true
	}
	return false
}

func emptyStatusEmojis() AlivenessEmojis {
	topMap := make(AlivenessEmojis)
	topMap[true] = make([]Emoji, 18) // 18 colors for alive/dead
	topMap[false] = make([]Emoji, 18)
	return topMap
}

func (bot *Bot) verifyEmojis(s *discordgo.Session, guildID string, alive bool, serverEmojis []*discordgo.Emoji, add bool) {
	for i, emoji := range GlobalAlivenessEmojis[alive] {
		alreadyExists := false
		for _, v := range serverEmojis {
			if v.Name == emoji.Name {
				emoji.ID = v.ID
				bot.StatusEmojis[alive][i] = emoji
				alreadyExists = true
				break
			}
		}
		if add && !alreadyExists {
			b64, err := emoji.DownloadAndBase64Encode()
			if err != nil {
				log.Printf("Failed to download emoji %s: %v", emoji.Name, err)
				continue
			}
			p := discordgo.EmojiParams{
				Name:  emoji.Name,
				Image: b64,
				Roles: nil,
			}
			em, err := s.GuildEmojiCreate(guildID, &p)
			if err != nil {
				log.Println(err)
			} else {
				log.Printf("Added emoji %s successfully!\n", emoji.Name)
				emoji.ID = em.ID
				bot.StatusEmojis[alive][i] = emoji
			}
		}
	}
}

func EmojisToSelectMenuOptions(emojis []Emoji, unlinkEmoji string) (arr []discordgo.SelectMenuOption) {
	for i, v := range emojis {
		arr = append(arr, v.toSelectMenuOption(game.GetColorStringForInt(i)))
	}
	arr = append(arr, discordgo.SelectMenuOption{
		Label:   "unlink",
		Value:   UnlinkEmojiName,
		Emoji:   discordgo.ComponentEmoji{Name: unlinkEmoji},
		Default: false,
	})
	return arr
}

func (e Emoji) toSelectMenuOption(displayName string) discordgo.SelectMenuOption {
	return discordgo.SelectMenuOption{
		Label:   displayName,
		Value:   displayName, // use the Name for listen events later
		Emoji:   discordgo.ComponentEmoji{ID: e.ID},
		Default: false,
	}
}

// AlivenessEmojis map
type AlivenessEmojis map[bool][]Emoji

// GlobalAlivenessEmojis keys are IsAlive, Color
var GlobalAlivenessEmojis = AlivenessEmojis{
	true: []Emoji{
		game.Red: {
			Name: "aured",
			ID:   "866558066921177108",
		},
		game.Blue: {
			Name: "aublue",
			ID:   "866558066484183060",
		},
		game.Green: {
			Name: "augreen",
			ID:   "866558066568986664",
		},
		game.Pink: {
			Name: "aupink",
			ID:   "866558067004538891",
		},
		game.Orange: {
			Name: "auorange",
			ID:   "866558066902958090",
		},
		game.Yellow: {
			Name: "auyellow",
			ID:   "866558067243221002",
		},
		game.Black: {
			Name: "aublack",
			ID:   "866558066442895370",
		},
		game.White: {
			Name: "auwhite",
			ID:   "866558067026165770",
		},
		game.Purple: {
			Name: "aupurple",
			ID:   "866558066966396928",
		},
		game.Brown: {
			Name: "aubrown",
			ID:   "866558066564136970",
		},
		game.Cyan: {
			Name: "aucyan",
			ID:   "866558066525601853",
		},
		game.Lime: {
			Name: "aulime",
			ID:   "866558066963382282",
		},
		game.Maroon: {
			Name: "aumaroon",
			ID:   "866558066917113886",
		},
		game.Rose: {
			Name: "aurose",
			ID:   "866558066921439242",
		},
		game.Banana: {
			Name: "aubanana",
			ID:   "866558065917558797",
		},
		game.Gray: {
			Name: "augray",
			ID:   "866558066174459905",
		},
		game.Tan: {
			Name: "autan",
			ID:   "866558066820382721",
		},
		game.Coral: {
			Name: "aucoral",
			ID:   "866558066552209448",
		},
	},
	false: []Emoji{
		game.Red: {
			Name: "aureddead",
			ID:   "866558067255279636",
		},
		game.Blue: {
			Name: "aubluedead",
			ID:   "866558066660999218",
		},
		game.Green: {
			Name: "augreendead",
			ID:   "866558067088949258",
		},
		game.Pink: {
			Name: "aupinkdead",
			ID:   "866558066945556512",
		},
		game.Orange: {
			Name: "auorangedead",
			ID:   "866558067508510730",
		},
		game.Yellow: {
			Name: "auyellowdead",
			ID:   "866558067206520862",
		},
		game.Black: {
			Name: "aublackdead",
			ID:   "866558066668339250",
		},
		game.White: {
			Name: "auwhitedead",
			ID:   "866558067231293450",
		},
		game.Purple: {
			Name: "aupurpledead",
			ID:   "866558067223298048",
		},
		game.Brown: {
			Name: "aubrowndead",
			ID:   "866558066945163304",
		},
		game.Cyan: {
			Name: "aucyandead",
			ID:   "866558067051200512",
		},
		game.Lime: {
			Name: "aulimedead",
			ID:   "866558067344408596",
		},
		game.Maroon: {
			Name: "aumaroondead",
			ID:   "866558067238895626",
		},
		game.Rose: {
			Name: "aurosedead",
			ID:   "866558067083444225",
		},
		game.Banana: {
			Name: "aubananadead",
			ID:   "866558066342625350",
		},
		game.Gray: {
			Name: "augraydead",
			ID:   "866558067049758740",
		},
		game.Tan: {
			Name: "autandead",
			ID:   "866558067230638120",
		},
		game.Coral: {
			Name: "aucoraldead",
			ID:   "866558067024723978",
		},
	},
}

/*
Helpful for copy/paste into Discord to get new emoji IDs when they are re-uploaded...
\:aured:
\:aublue:
\:augreen:
\:aupink:
\:auorange:
\:auyellow:
\:aublack:
\:auwhite:
\:aupurple:
\:aubrown:
\:aucyan:
\:aulime:
\:aumaroon:
\:aurose:
\:aubanana:
\:augray:
\:autan:
\:aucoral:

\:aureddead:
\:aubluedead:
\:augreendead:
\:aupinkdead:
\:auorangedead:
\:auyellowdead:
\:aublackdead:
\:auwhitedead:
\:aupurpledead:
\:aubrowndead:
\:aucyandead:
\:aulimedead:
\:aumaroondead:
\:aurosedead:
\:aubananadead:
\:augraydead:
\:autandead:
\:aucoraldead:
*/
