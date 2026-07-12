package tokenprovider

import (
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

func TestWaitForAckMessageAcceptsSuccessfulAck(t *testing.T) {
	channel := make(chan *redis.Message, 1)
	channel <- &redis.Message{Payload: "true"}

	if !waitForAckMessage(channel, time.Second) {
		t.Fatal("expected a true ACK to succeed")
	}
}

func TestWaitForAckMessageRejectsNegativeAck(t *testing.T) {
	channel := make(chan *redis.Message, 1)
	channel <- &redis.Message{Payload: "false"}

	if waitForAckMessage(channel, time.Second) {
		t.Fatal("expected a non-true ACK to fail")
	}
}

func TestWaitForAckMessageHandlesClosedChannel(t *testing.T) {
	channel := make(chan *redis.Message)
	close(channel)

	if waitForAckMessage(channel, time.Second) {
		t.Fatal("expected a closed ACK channel to fail safely")
	}
}

func TestWaitForAckMessageHandlesNilMessage(t *testing.T) {
	channel := make(chan *redis.Message, 1)
	channel <- nil

	if waitForAckMessage(channel, time.Second) {
		t.Fatal("expected a nil ACK message to fail safely")
	}
}

func TestWaitForAckMessageTimesOut(t *testing.T) {
	channel := make(chan *redis.Message)

	if waitForAckMessage(channel, 10*time.Millisecond) {
		t.Fatal("expected a missing ACK to time out")
	}
}
