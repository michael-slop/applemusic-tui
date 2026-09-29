package fx

import (
	"strconv"
	"strings"
	"sync"

	"github.com/muesli/termenv"
)

// Render draws an effect into exactly rows lines of exactly cols cells each,
// with ANSI colour. Consecutive cells of the same colour share one escape
// sequence, which is what keeps a full-panel animation cheap at 30 fps (the
// panefx renderer's lesson: the drawing, not the simulation, is the cost).
//
// profile is the terminal's colour profile (lipgloss.ColorProfile()); colours
// are converted to it, so 256-colour terminals still work.
func Render(e Effect, cols, rows int, profile termenv.Profile) string {
	if cols <= 0 || rows <= 0 {
		return ""
	}
	var sb strings.Builder
	sb.Grow(rows * (cols + 16))
	plain := profile == termenv.Ascii // no colour at all: no resets either
	for r := 0; r < rows; r++ {
		if r > 0 {
			sb.WriteByte('\n')
		}
		var cur RGB
		open := false
		for c := 0; c < cols; c++ {
			ch, col, ok := e.Cell(c, r)
			if !ok || ch == 0 || ch == ' ' {
				if open && !plain {
					sb.WriteString(reset)
					open = false
				}
				sb.WriteByte(' ')
				continue
			}
			if !plain && (!open || col != cur) {
				sb.WriteString(fgSeq(col, profile))
				cur, open = col, true
			}
			sb.WriteRune(ch)
		}
		if open && !plain {
			sb.WriteString(reset)
		}
	}
	return sb.String()
}

const reset = "\x1b[0m"

type seqKey struct {
	c RGB
	p termenv.Profile
}

var (
	seqMu    sync.Mutex
	seqCache = map[seqKey]string{}
)

// fgSeq is the escape sequence that sets the foreground to c, cached.
func fgSeq(c RGB, p termenv.Profile) string {
	k := seqKey{c, p}
	seqMu.Lock()
	s, ok := seqCache[k]
	seqMu.Unlock()
	if ok {
		return s
	}
	if p == termenv.Ascii {
		s = ""
	} else {
		hex := "#" + hex2(c.R) + hex2(c.G) + hex2(c.B)
		s = termenv.CSI + p.Color(hex).Sequence(false) + "m"
	}
	seqMu.Lock()
	if len(seqCache) > 4096 {
		clear(seqCache)
	}
	seqCache[k] = s
	seqMu.Unlock()
	return s
}

func hex2(b uint8) string {
	s := strconv.FormatUint(uint64(b), 16)
	if len(s) == 1 {
		return "0" + s
	}
	return s
}
