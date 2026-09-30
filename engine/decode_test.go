package engine

import (
	"testing"
	"unicode/utf8"
)

// Apple Music served these decomposed (letter + combining mark); on Windows'
// inbox console each mark took a cell, so a full-width row wrapped and the TUI
// scrolled every frame.
func TestDecodeJSONComposesDecomposedTitles(t *testing.T) {
	raw := `{"title":"Dança","artist":"M.A.D.S FORÇA E UNIÃO"}`
	var got struct{ Title, Artist string }
	if err := decodeJSON(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != "Dança" || got.Artist != "M.A.D.S FORÇA E UNIÃO" {
		t.Fatalf("decoded %q / %q, want the composed forms", got.Title, got.Artist)
	}
	if n := utf8.RuneCountInString(got.Title); n != 5 {
		t.Fatalf("title has %d code points, want 5 (ç as one)", n)
	}
}

func TestDecodeJSONKeepsComposedTextAndErrors(t *testing.T) {
	var got struct{ Title string }
	if err := decodeJSON(`{"title":"Судно — ç"}`, &got); err != nil || got.Title != "Судно — ç" {
		t.Fatalf("composed text changed: %q, %v", got.Title, err)
	}
	if err := decodeJSON(`{"title":`, &got); err == nil {
		t.Fatal("malformed JSON must still fail")
	}
}
