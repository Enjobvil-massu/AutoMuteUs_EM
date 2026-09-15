package bot

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// StaleHeartbeat is how long the Discord gateway may go without acknowledging
// a heartbeat before the shard is reported unhealthy.
const StaleHeartbeat = 2 * time.Minute

// GatewayHealth reports whether this shard's gateway session is connected and
// still receiving heartbeat acknowledgements from Discord.
func (bot *Bot) GatewayHealth(context.Context) error {
	if bot == nil || bot.PrimarySession == nil {
		return errors.New("gateway session unavailable")
	}

	s := bot.PrimarySession
	s.RLock()
	ready := s.DataReady
	lastAck := s.LastHeartbeatAck
	s.RUnlock()

	if !ready {
		return errors.New("gateway not connected")
	}
	if lastAck.IsZero() {
		return errors.New("heartbeat not yet acknowledged")
	}
	if age := time.Since(lastAck); age > StaleHeartbeat {
		return fmt.Errorf("no heartbeat ack for %s", age.Truncate(time.Second))
	}
	return nil
}
