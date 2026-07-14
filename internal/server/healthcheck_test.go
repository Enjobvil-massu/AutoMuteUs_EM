package server

import (
	"testing"
	"time"
)

func TestHealthCheckHTTPClientTimeout(t *testing.T) {
	if healthCheckHTTPClient == nil {
		t.Fatal("healthCheckHTTPClient must not be nil")
	}

	const want = 10 * time.Second
	if got := healthCheckHTTPClient.Timeout; got != want {
		t.Fatalf("healthCheckHTTPClient.Timeout = %v, want %v", got, want)
	}
}
