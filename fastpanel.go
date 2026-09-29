package main

import (
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The view is rebuilt up to 30 times a second and most of its lines (cover
// art rows, lyrics, queue entries) are byte-identical between frames. lipgloss
// re-measures every line several times per Render, which parses every ANSI
// escape each time; that measuring was most of amtui's remaining CPU.
// cellWidth remembers widths of lines it has already seen.

const widthCacheMax = 2048

var (
	widthMu    sync.Mutex
	widthCache = map[string]int{}
)

func cellWidth(s string) int {
	if len(s) < 16 { // short strings: measuring beats hashing
		return ansi.StringWidth(s)
	}
	widthMu.Lock()
	w, ok := widthCache[s]
	widthMu.Unlock()
	if ok {
		return w
	}
	w = ansi.StringWidth(s)
	widthMu.Lock()
	if len(widthCache) >= widthCacheMax {
		clear(widthCache)
	}
	widthCache[s] = w
	widthMu.Unlock()
	return w
}

// borderedBox renders content like
//
//	lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(bc).
//		Width(w).Height(h).Render(content)
//
// for the common case where no line is wider than w, measuring each line once
// through the cache. Anything else (a line that would need wrapping, tabs)
// goes to lipgloss unchanged, so output is always what lipgloss would produce.
func borderedBox(content string, w, h int, bc lipgloss.TerminalColor) string {
	slow := func() string {
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).BorderForeground(bc).
			Width(w).Height(h).Render(content)
	}
	if w <= 0 || strings.ContainsAny(content, "\t\r") {
		return slow()
	}
	lines := strings.Split(content, "\n")
	widths := make([]int, len(lines))
	for i, l := range lines {
		widths[i] = cellWidth(l)
		if widths[i] > w {
			return slow()
		}
	}
	for len(lines) < h {
		lines = append(lines, "")
		widths = append(widths, 0)
	}
	b := lipgloss.RoundedBorder()
	edge := lipgloss.NewStyle().Foreground(bc)
	side := edge.Render(b.Left)
	right := edge.Render(b.Right)
	var sb strings.Builder
	sb.Grow(len(content) + (w+8)*(len(lines)+2))
	sb.WriteString(edge.Render(b.TopLeft + strings.Repeat(b.Top, w) + b.TopRight))
	for i, l := range lines {
		sb.WriteByte('\n')
		sb.WriteString(side)
		sb.WriteString(l)
		sb.WriteString(strings.Repeat(" ", w-widths[i]))
		sb.WriteString(right)
	}
	sb.WriteByte('\n')
	sb.WriteString(edge.Render(b.BottomLeft + strings.Repeat(b.Bottom, w) + b.BottomRight))
	return sb.String()
}

// joinVerticalLeft is lipgloss.JoinVertical(lipgloss.Left, blocks...) with
// cached line widths: every line is padded to the widest line of all blocks.
func joinVerticalLeft(blocks ...string) string {
	var lines []string
	for _, b := range blocks {
		lines = append(lines, strings.Split(b, "\n")...)
	}
	widths := make([]int, len(lines))
	maxW := 0
	for i, l := range lines {
		widths[i] = cellWidth(l)
		maxW = max(maxW, widths[i])
	}
	var sb strings.Builder
	for i, l := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(l)
		sb.WriteString(strings.Repeat(" ", maxW-widths[i]))
	}
	return sb.String()
}

// joinHorizontalTop is lipgloss.JoinHorizontal(lipgloss.Top, blocks...) with
// cached line widths: blocks sit side by side, shorter ones padded with blank
// rows at the bottom, each block's lines padded to that block's width.
func joinHorizontalTop(blocks ...string) string {
	if len(blocks) == 0 {
		return ""
	}
	cols := make([][]string, len(blocks))
	colW := make([]int, len(blocks))
	rows := 0
	for i, b := range blocks {
		cols[i] = strings.Split(b, "\n")
		for _, l := range cols[i] {
			colW[i] = max(colW[i], cellWidth(l))
		}
		rows = max(rows, len(cols[i]))
	}
	var sb strings.Builder
	for r := 0; r < rows; r++ {
		if r > 0 {
			sb.WriteByte('\n')
		}
		for i, col := range cols {
			l := ""
			if r < len(col) {
				l = col[r]
			}
			sb.WriteString(l)
			sb.WriteString(strings.Repeat(" ", colW[i]-cellWidth(l)))
		}
	}
	return sb.String()
}
