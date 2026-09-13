package task

import (
	"encoding/hex"
	"testing"
)

func TestNewModifyTaskUsesUniqueRandomIDs(t *testing.T) {
	const samples = 256

	params := PatchParams{
		Deaf: false,
		Mute: true,
	}
	seen := make(map[string]struct{}, samples)

	for i := 0; i < samples; i++ {
		got := NewModifyTask(123456789, 987654321, params)

		if got.GuildID != 123456789 {
			t.Fatalf("GuildID = %d, want %d", got.GuildID, uint64(123456789))
		}
		if got.UserID != 987654321 {
			t.Fatalf("UserID = %d, want %d", got.UserID, uint64(987654321))
		}
		if got.Parameters != params {
			t.Fatalf("Parameters = %+v, want %+v", got.Parameters, params)
		}
		if len(got.TaskID) != 16 {
			t.Fatalf("TaskID length = %d, want 16: %q", len(got.TaskID), got.TaskID)
		}
		if _, err := hex.DecodeString(got.TaskID); err != nil {
			t.Fatalf("TaskID is not valid hex: %q: %v", got.TaskID, err)
		}
		if _, exists := seen[got.TaskID]; exists {
			t.Fatalf("duplicate TaskID generated: %q", got.TaskID)
		}
		seen[got.TaskID] = struct{}{}
	}
}
