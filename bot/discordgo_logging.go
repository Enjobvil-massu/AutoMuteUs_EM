package bot

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

// unknownEventPrefix is the start of the message discordgo logs (at LogWarning)
// for gateway events it has no struct for. Discord adds events the bot does not
// use, and dumping each payload creates noisy logs.
const unknownEventPrefix = "unknown event:"

// droppedMessages are informational bookkeeping messages emitted on every
// open/close/reconnect. Useful reconnect, REST retry and rate-limit messages are
// deliberately retained.
var droppedMessages = map[string]struct{}{
	"called":                                   {},
	"exiting":                                  {},
	"creating new VoiceConnections map":        {},
	"closing listening channel":                {},
	"sending close frame":                      {},
	"closing gateway websocket":                {},
	"emit disconnect event":                    {},
	"Op 10 Hello Packet received from Discord": {},
}

var installDiscordgoLoggerOnce sync.Once

// installDiscordgoLogger routes discordgo's logging through discordgoLogger. It
// is safe to call multiple times; only the first call has an effect.
func installDiscordgoLogger() {
	installDiscordgoLoggerOnce.Do(func() {
		discordgo.Logger = discordgoLogger
	})
}

// discordgoLogger mirrors discordgo's default log format, except selected noise
// is dropped. Session log levels remain authoritative.
func discordgoLogger(msgL, caller int, format string, a ...interface{}) {
	if shouldDropDiscordgoMessage(msgL, format) {
		return
	}

	pc, file, line, _ := runtime.Caller(caller + 1)
	files := strings.Split(file, "/")
	file = files[len(files)-1]

	name := "unknown"
	if fn := runtime.FuncForPC(pc); fn != nil {
		parts := strings.Split(fn.Name(), ".")
		name = parts[len(parts)-1]
	}

	msg := fmt.Sprintf(format, a...)
	log.Printf("[DG%d] %s:%d:%s() %s\n", msgL, file, line, name, msg)
}

func shouldDropDiscordgoMessage(msgL int, format string) bool {
	if _, ok := droppedMessages[format]; ok {
		return true
	}
	return msgL == discordgo.LogWarning && strings.HasPrefix(format, unknownEventPrefix)
}
