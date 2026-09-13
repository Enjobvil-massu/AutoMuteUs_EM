package tokenprovider

import "testing"

func TestCaptureRouteUsableUntilFirstDeadTransition(t *testing.T) {
	route := &captureRoute{available: true}

	if !route.usable() {
		t.Fatal("expected a newly available Capture route to be usable")
	}

	if !route.markDead() {
		t.Fatal("expected first dead transition to succeed")
	}

	if route.usable() {
		t.Fatal("expected a dead Capture route to stop being usable")
	}

	if route.markDead() {
		t.Fatal("expected repeated dead transition to be ignored")
	}
}

func TestCaptureRouteUnavailableIsNeverUsable(t *testing.T) {
	route := &captureRoute{available: false}

	if route.usable() {
		t.Fatal("expected an unavailable Capture route to be unusable")
	}
}

func TestNilCaptureRouteIsNotUsable(t *testing.T) {
	var route *captureRoute

	if route.usable() {
		t.Fatal("expected a nil Capture route to be unusable")
	}

	if route.markDead() {
		t.Fatal("expected a nil Capture route dead transition to be ignored")
	}
}
