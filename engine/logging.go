package engine

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// chromedp logs through log.Printf by default, which writes to stderr — i.e.
// straight over the TUI. Chrome 154 started sending DOM.topLayerElementsUpdated,
// which chromedp v0.16 does not know, so every page emitted
// "unhandled node event *dom.EventTopLayerElementsUp..." into the play bar.
// Everything chromedp says goes to a small log file instead, and the benign
// "unhandled event" class is recorded once per event type.

const logMaxBytes = 1 << 20

var (
	logMu   sync.Mutex
	logFile *os.File
	seen    = map[string]bool{}
)

// LogPath is where amtui's diagnostic log lives.
func LogPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "amtui", "amtui.log")
}

// OpenLog routes the standard logger (and chromedp) to the log file. Call it
// before the TUI starts; anything printed to stderr afterwards corrupts it.
func OpenLog() {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		return
	}
	p := LogPath()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if st, err := os.Stat(p); err == nil && st.Size() > logMaxBytes {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	f, err := os.OpenFile(p, flags, 0o644)
	if err != nil {
		log.SetOutput(discard{})
		return
	}
	logFile = f
	log.SetOutput(f)
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// chromeLog is chromedp's logf/errorf. Unknown-event noise is deduplicated.
func chromeLog(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if strings.HasPrefix(msg, "unhandled ") || strings.Contains(msg, ": unhandled ") {
		key := msg
		if i := strings.Index(key, " *"); i >= 0 {
			key = key[:i] + " " + strings.Fields(key[i:])[0]
		}
		logMu.Lock()
		dup := seen[key]
		seen[key] = true
		logMu.Unlock()
		if dup {
			return
		}
		msg += " (logged once)"
	}
	log.Printf("chromedp %s: %s", time.Now().Format("15:04:05"), msg)
}
