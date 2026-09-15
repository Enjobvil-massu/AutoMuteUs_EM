package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/automuteus/automuteus/v8/storage"
	"github.com/bwmarrin/discordgo"
)

func gatewayBot(t *testing.T) *Bot {
	t.Helper()
	sess, err := discordgo.New("Bot fake")
	if err != nil {
		t.Fatal(err)
	}
	return &Bot{PrimarySession: sess}
}

func TestGatewayHealth_MissingSession(t *testing.T) {
	var nilBot *Bot
	if err := nilBot.GatewayHealth(context.Background()); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil bot err = %v", err)
	}
	if err := (&Bot{}).GatewayHealth(context.Background()); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("empty bot err = %v", err)
	}
}

func TestGatewayHealth_NotConnected(t *testing.T) {
	bot := gatewayBot(t)
	if err := bot.GatewayHealth(context.Background()); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("err = %v", err)
	}
}

func TestGatewayHealth_ConnectedButNotAcked(t *testing.T) {
	bot := gatewayBot(t)
	bot.PrimarySession.DataReady = true
	if err := bot.GatewayHealth(context.Background()); err == nil || !strings.Contains(err.Error(), "not yet acknowledged") {
		t.Fatalf("err = %v", err)
	}
}

func TestGatewayHealth_ConnectedAndAcked(t *testing.T) {
	bot := gatewayBot(t)
	bot.PrimarySession.DataReady = true
	bot.PrimarySession.LastHeartbeatAck = time.Now().Add(-30 * time.Second)
	if err := bot.GatewayHealth(context.Background()); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestGatewayHealth_StaleHeartbeat(t *testing.T) {
	bot := gatewayBot(t)
	bot.PrimarySession.DataReady = true
	bot.PrimarySession.LastHeartbeatAck = time.Now().Add(-(StaleHeartbeat + time.Minute))
	err := bot.GatewayHealth(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no heartbeat ack for 3m") {
		t.Fatalf("err = %v", err)
	}
}

func TestGatewayHealth_DisconnectWinsOverRecentAck(t *testing.T) {
	bot := gatewayBot(t)
	bot.PrimarySession.DataReady = false
	bot.PrimarySession.LastHeartbeatAck = time.Now()
	if err := bot.GatewayHealth(context.Background()); err == nil {
		t.Fatal("expected not connected")
	}
}

func TestRedisPing_ReportsReachability(t *testing.T) {
	mr := miniredis.RunT(t)
	ri := &RedisInterface{}
	if err := ri.Init(storage.RedisParameters{Addr: mr.Addr()}); err != nil {
		t.Fatal(err)
	}
	defer ri.Close()

	if err := ri.Ping(context.Background()); err != nil {
		t.Fatalf("ping against a live server: %v", err)
	}
	mr.Close()
	if err := ri.Ping(context.Background()); err == nil {
		t.Fatal("ping should fail once Redis is gone")
	}
}

func TestRedisPing_NilClientFailsClosed(t *testing.T) {
	if err := (&RedisInterface{}).Ping(context.Background()); err == nil {
		t.Fatal("expected nil Redis client to fail readiness")
	}
}
