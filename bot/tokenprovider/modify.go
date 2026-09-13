package tokenprovider

import (
	"context"
	"encoding/json"
	"github.com/automuteus/automuteus/v8/internal/server"
	"github.com/automuteus/automuteus/v8/pkg/rediskey"
	"github.com/automuteus/automuteus/v8/pkg/task"
	"github.com/go-redis/redis/v8"
	"log"
	"sync/atomic"
)

func RecordDiscordRequestsByCounts(client *redis.Client, counts task.MuteDeafenSuccessCounts) {
	server.RecordDiscordRequests(client, server.MuteDeafenOfficial, counts.Official)
	server.RecordDiscordRequests(client, server.MuteDeafenWorker, counts.Worker)
	server.RecordDiscordRequests(client, server.MuteDeafenCapture, counts.Capture)
	server.RecordDiscordRequests(client, server.InvalidRequest, counts.RateLimit)
}

// captureRoute is shared by all workers in one ModifyUsers batch.
// It intentionally does not require CaptureMuteReady yet so EM remains compatible with legacy Galactus.
type captureRoute struct {
	guildID     string
	connectCode string
	available   bool
	dead        atomic.Bool
}

func (tokenProvider *TokenProvider) newCaptureRoute(guildID, connectCode string) *captureRoute {
	route := &captureRoute{
		guildID:     guildID,
		connectCode: connectCode,
		available:   true,
	}
	if tokenProvider.isBlacklisted(guildID, connectCode) {
		route.available = false
		log.Printf("Capture client for gamecode %q is blacklisted. Using bot fallback for this batch", connectCode)
	}
	return route
}

func (route *captureRoute) usable() bool {
	return route != nil && route.available && !route.dead.Load()
}

// markDead returns true only for the first worker that transitions this batch to dead.
func (route *captureRoute) markDead() bool {
	if route == nil {
		return false
	}
	return route.dead.CompareAndSwap(false, true)
}
func (tokenProvider *TokenProvider) attemptOnSecondaryTokens(guildID, userID string, tokenSubset map[string]struct{}, request task.UserModify) string {
	if len(tokenProvider.activeSessions) > 0 {
		sess, hToken := tokenProvider.getSession(guildID, tokenSubset)
		if sess != nil {
			err := task.ApplyMuteDeaf(sess, guildID, userID, request.Mute, request.Deaf)
			if err != nil {
				log.Println("Failed to apply mute to player with error:")
				log.Println(err)

				// don't attempt this token for this guild for another 5 minutes
				err = tokenProvider.BlacklistTokenForDuration(guildID, hToken, UnresponsiveCaptureBlacklistDuration)
				if err != nil {
					log.Println(err)
				}
			} else {
				log.Printf("Successfully applied mute=%v, deaf=%v to User %d using secondary bot: %s\n", request.Mute, request.Deaf, request.UserID, hToken)
				return hToken
			}
		} else {
			log.Println("No secondary bot tokens found. Trying other methods")
		}
	} else {
		log.Println("Guild has no access to secondary bot tokens; skipping")
	}
	return ""
}

func (tokenProvider *TokenProvider) attemptOnCaptureBot(route *captureRoute, gid uint64, request task.UserModify) bool {
	if !route.usable() {
		return false
	}

	guildID := route.guildID
	connectCode := route.connectCode

	// this is cheeky, but use the connect code as part of the lock; don't issue too many requests on the capture client w/ this code
	if tokenProvider.IncrAndTestGuildTokenComboLock(guildID, connectCode) {
		// if the secondary token didn't work, then next we try the client-side capture request
		taskObj := task.NewModifyTask(gid, request.UserID, task.PatchParams{
			Deaf: request.Deaf,
			Mute: request.Mute,
		})
		jBytes, err := json.Marshal(taskObj)
		if err != nil {
			log.Println(err)
			return false
		}
		// Subscribe before publishing the task. go-redis Subscribe returns before
		// Redis confirms the subscription, so Receive prevents a fast ACK from
		// being missed between Subscribe and Publish.
		ackContext, cancel := context.WithTimeout(context.Background(), tokenProvider.taskTimeoutMs)
		defer cancel()

		pubsub := tokenProvider.client.Subscribe(ackContext, rediskey.CompleteTask(taskObj.TaskID))
		defer func() {
			if closeErr := pubsub.Close(); closeErr != nil {
				log.Printf("Error closing capture ACK subscription for task %s: %v", taskObj.TaskID, closeErr)
			}
		}()

		if _, receiveErr := pubsub.Receive(ackContext); receiveErr != nil {
			log.Printf("Unable to establish capture ACK subscription for task %s: %v", taskObj.TaskID, receiveErr)
			return false
		}

		err = tokenProvider.client.Publish(ackContext, rediskey.TasksList(connectCode), jBytes).Err()
		if err != nil {
			log.Println("Error in publishing task to " + rediskey.TasksList(connectCode))
			log.Println(err)
		} else {
			res := waitForAckMessage(pubsub.Channel(), tokenProvider.taskTimeoutMs)
			if res {
				log.Println("Successful mute/deafen using client capture bot!")

				// hooray! we did the mute with a client token!
				return true
			}
			if route.markDead() {
				err = tokenProvider.BlacklistTokenForDuration(guildID, connectCode, UnresponsiveCaptureBlacklistDuration)
				if err == nil {
					log.Printf("No ack from capture client; marking batch route dead and blacklisting gamecode \"%s\" for %s\n", connectCode, UnresponsiveCaptureBlacklistDuration.String())
				} else {
					log.Printf("Unable to persist Capture blacklist for gamecode %q: %v", connectCode, err)
				}
			}
		}

	} else {
		log.Println("Capture client is probably rate-limited. Deferring to main bot instead")
	}
	return false
}
