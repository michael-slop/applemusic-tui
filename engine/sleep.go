package engine

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// Snapshot is everything needed to rebuild the player after the browser has
// been closed to save memory: the whole queue (song ids, catalog or library),
// where in it we were, and the player settings.
type Snapshot struct {
	IDs     []string `json:"ids"`
	Pos     int      `json:"pos"`
	Time    float64  `json:"t"`
	Volume  float64  `json:"volume"`
	Shuffle int      `json:"shuffle"`
	Repeat  int      `json:"repeat"`
}

// Empty reports whether there is no queue to restore.
func (s Snapshot) Empty() bool { return len(s.IDs) == 0 }

const snapshotJS = `(() => {
  const mk = MusicKit.getInstance();
  const q = mk.queue;
  return JSON.stringify({
    ids: q ? q.items.map((x) => String((x && x.id) || '')).filter(Boolean) : [],
    pos: q ? Math.max(0, q.position | 0) : 0,
    t: mk.currentPlaybackTime || 0,
    volume: typeof mk.volume === 'number' ? mk.volume : 1,
    shuffle: mk.shuffleMode || 0,
    repeat: mk.repeatMode || 0,
  });
})()`

// restoreJS rebuilds the queue. setQueue({songs}) resolves both catalog and
// library ("i.…") ids. When playing, it seeks back to the saved time once
// playback has started; the promise chain is fire-and-forget like every other
// action (errors surface through the next state poll).
const restoreJS = `(() => {
  ` + trapJS + playerWaitJS + `
  const mk = MusicKit.getInstance();
  const s = %s;
  const play = %t;
  const run = async () => {
    try { mk.volume = s.volume; } catch (e) {}
    try { mk.repeatMode = s.repeat; } catch (e) {}
    await mk.setQueue({ songs: s.ids, startWith: s.pos, startPlaying: play });
    try { mk.shuffleMode = s.shuffle; } catch (e) {}
    if (play && s.t > 1) {
      await waitFor(() => mk.playbackState === 2, 20000, 'restored playback');
      await mk.seekToTime(s.t);
    }
  };
  trap('restore')(run());
  return true;
})()`

// Snapshot captures the queue and position for a later Restore.
func (e *Engine) Snapshot() (Snapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var s Snapshot
	err := e.evalJSONLocked(snapshotJS, &s)
	return s, err
}

// Restore loads a snapshot into a freshly connected browser.
func (e *Engine) Restore(s Snapshot, play bool) error {
	if s.Empty() {
		return nil
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return e.do(fmt.Sprintf(restoreJS, string(raw), play))
}

// Sleep closes the browser gracefully (so Chrome flushes its profile, keeping
// the session) and falls back to a hard close if that takes too long.
func (e *Engine) Sleep() {
	e.mu.Lock()
	defer e.mu.Unlock()
	done := make(chan struct{})
	go func() {
		_ = chromedp.Cancel(e.ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	if c := chromedp.FromContext(e.ctx); c != nil && c.Browser != nil {
		if p := c.Browser.Process(); p != nil {
			_ = p.Kill() // no-op if it already exited
		}
	}
	closeAll(e.cancels)
}
