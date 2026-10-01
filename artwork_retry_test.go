package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/k1y0miiii/applemusic-tui/engine"
)

// coverServer answers with the given statuses in turn (the last one repeats);
// 200 serves a small PNG, or body when it is set.
func coverServer(t *testing.T, body []byte, statuses ...int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	orig := artworkRetryDelays
	artworkRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { artworkRetryDelays = orig })
	if body == nil {
		var buf bytes.Buffer
		img := image.NewRGBA(image.Rect(0, 0, 4, 4))
		img.Set(1, 1, color.RGBA{200, 50, 50, 255})
		png.Encode(&buf, img)
		body = buf.Bytes()
	}
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		i := int(n.Add(1)) - 1
		st := statuses[min(i, len(statuses)-1)]
		w.WriteHeader(st)
		if st == http.StatusOK {
			w.Write(body)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func TestArtworkIsAskedForAgainAfterARefusal(t *testing.T) {
	srv, n := coverServer(t, nil, 403, 200)
	img, err := fetchArtwork(context.Background(), srv.URL)
	if err != nil || img == nil {
		t.Fatalf("cover refused once then served: img=%v err=%v", img, err)
	}
	if got := n.Load(); got != 2 {
		t.Fatalf("%d requests, want 2", got)
	}
}

func TestArtworkRefusalIsNamedNotDecoded(t *testing.T) {
	srv, n := coverServer(t, nil, 403)
	_, err := fetchArtwork(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Fatalf("err = %v, want it to name HTTP 403 (it used to say 'unknown format')", err)
	}
	if got := n.Load(); got != int32(1+len(artworkRetryDelays)) {
		t.Fatalf("%d requests, want %d", got, 1+len(artworkRetryDelays))
	}
}

func TestArtworkThatIsNotAnImageIsNotRetried(t *testing.T) {
	srv, n := coverServer(t, []byte("<html>not a picture</html>"), 200)
	if _, err := fetchArtwork(context.Background(), srv.URL); err == nil {
		t.Fatal("a page that is not an image decoded")
	}
	if got := n.Load(); got != 1 {
		t.Fatalf("%d requests for a non-image, want 1", got)
	}
}

func TestFailedTileArtIsFetchedAgain(t *testing.T) {
	m := model{
		recent: []engine.Track{
			{ID: "failed", Art: "x/{w}x{h}bb.jpg"},
			{ID: "loaded", Art: "y/{w}x{h}bb.jpg"},
			{ID: "new", Art: "z/{w}x{h}bb.jpg"},
			{ID: "no-art"},
		},
		tileArt: map[string]image.Image{
			"failed": nil, // the old "tried and failed" marker
			"loaded": image.NewRGBA(image.Rect(0, 0, 1, 1)),
		},
	}
	if got := len(m.fetchTileArtCmds()); got != 2 {
		t.Fatalf("%d fetches, want 2: the failed tile and the new one, not the loaded one", got)
	}
}
