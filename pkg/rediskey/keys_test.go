package rediskey

import "testing"

func TestMuteBlacklist(t *testing.T) {
	got := MuteBlacklist("123456", "ABCDEFGH")
	want := "automuteus:muterequest:blacklist:ABCDEFGH:123456"

	if got != want {
		t.Fatalf("MuteBlacklist() = %q, want %q", got, want)
	}
}

func TestCaptureMuteReady(t *testing.T) {
	got := CaptureMuteReady("ABCDEFGH")
	want := "automuteus:capture:muteready:ABCDEFGH"

	if got != want {
		t.Fatalf("CaptureMuteReady() = %q, want %q", got, want)
	}
}
