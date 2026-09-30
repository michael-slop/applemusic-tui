package lyrics

import (
	"testing"
	"time"
)

func TestParseLRCAndCurrent(t *testing.T) {
	src := "[ar:Artist]\n[00:12.50] first line\n[00:05] early line\n\nnot a tag\n[01:02.1] later"
	ly := ParseLRC(src)
	if !ly.Synced || len(ly.Lines) != 3 {
		t.Fatalf("want 3 synced lines, got %+v", ly)
	}
	if ly.Lines[0].Text != "early line" || ly.Lines[0].At != 5*time.Second {
		t.Errorf("bad sort/parse: %+v", ly.Lines[0])
	}
	if ly.Lines[1].At != 12500*time.Millisecond {
		t.Errorf("bad fraction: %v", ly.Lines[1].At)
	}
	if got := ly.Current(13 * time.Second); got != 1 {
		t.Errorf("Current(13s) = %d, want 1", got)
	}
	if got := ly.Current(time.Second); got != -1 {
		t.Errorf("Current(1s) = %d, want -1", got)
	}
}

func TestPickPrefersSyncedWithCloseDuration(t *testing.T) {
	rs := []result{
		{Duration: 500, SyncedLyrics: "[00:01] far duration"},
		{Duration: 201, PlainLyrics: "plain close"},
		{Duration: 200, SyncedLyrics: "[00:01] synced close"},
	}
	best := pick(rs, 200)
	if best == nil || best.SyncedLyrics != "[00:01] synced close" {
		t.Errorf("pick chose %+v", best)
	}
}

func TestLyricsAreComposed(t *testing.T) {
	synced := ParseLRC("[00:01.00] Danc\u0327a")
	if len(synced.Lines) != 1 || synced.Lines[0].Text != "Dança" {
		t.Fatalf("synced line %+v, want the composed \"Dança\"", synced.Lines)
	}
	un := plain("FORC\u0327A E UNIA\u0303O")
	if len(un.Lines) != 1 || un.Lines[0].Text != "FORÇA E UNIÃO" {
		t.Fatalf("plain line %+v, want the composed form", un.Lines)
	}
}
