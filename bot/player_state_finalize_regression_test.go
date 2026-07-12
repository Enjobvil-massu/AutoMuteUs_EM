package bot

import (
	"reflect"
	"testing"
)

func TestFinalizePlayerStateUpdateSavesBeforeDispatch(t *testing.T) {
	var order []string
	finalizePlayerStateUpdate(
		func() { order = append(order, "save") },
		func() { order = append(order, "refresh") },
		func() { order = append(order, "dispatch") },
		false,
		true,
	)

	want := []string{"save", "dispatch"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestFinalizePlayerStateUpdatePrefersInitialRefreshAfterSave(t *testing.T) {
	var order []string
	finalizePlayerStateUpdate(
		func() { order = append(order, "save") },
		func() { order = append(order, "refresh") },
		func() { order = append(order, "dispatch") },
		true,
		true,
	)

	want := []string{"save", "refresh"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestFinalizePlayerStateUpdateOnlySavesWhenNoMessageUpdateRequested(t *testing.T) {
	var order []string
	finalizePlayerStateUpdate(
		func() { order = append(order, "save") },
		func() { order = append(order, "refresh") },
		func() { order = append(order, "dispatch") },
		false,
		false,
	)

	want := []string{"save"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestFinalizePlayerStateUpdateHandlesOptionalCallbacks(t *testing.T) {
	finalizePlayerStateUpdate(nil, nil, nil, true, true)
	finalizePlayerStateUpdate(nil, nil, nil, false, true)
}
