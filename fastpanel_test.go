package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestBorderedBoxMatchesLipgloss(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	red := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Bold(true)
	cell := lipgloss.NewStyle().Foreground(lipgloss.Color("#102030")).Background(lipgloss.Color("#405060")).Render("▀")
	contents := []string{
		"",
		"plain",
		pad(red.Render(" LYRICS"), 20) + "\nsecond line\n" + strings.Repeat(cell, 20),
		"wide 漢字 and emoji ⏸ ok\n" + red.Render("x"),
		strings.Repeat("a\n", 3) + "b",
		strings.Repeat("y", 20),
		"too long for the box, so lipgloss has to wrap this line", // slow path
		"tab\there",
	}
	for _, bc := range []lipgloss.TerminalColor{lipgloss.Color("#89b4fa"), lipgloss.Color("240")} {
		for _, c := range contents {
			for _, size := range [][2]int{{20, 1}, {20, 6}, {24, 2}} {
				w, h := size[0], size[1]
				want := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
					BorderForeground(bc).Width(w).Height(h).Render(c)
				got := borderedBox(c, w, h, bc)
				if got != want {
					t.Fatalf("w=%d h=%d content=%q\n got: %q\nwant: %q", w, h, c, got, want)
				}
			}
		}
	}
}

func TestJoinsMatchLipgloss(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000"))
	blocks := []string{
		"a\nbb\nccc",
		red.Render("colored") + "\n" + "x",
		"漢字\n⏸\n\n\nlast",
		"",
		borderedBox("inside a box", 14, 3, lipgloss.Color("#89b4fa")),
	}
	for i := range blocks {
		for j := range blocks {
			if got, want := joinVerticalLeft(blocks[i], blocks[j]), lipgloss.JoinVertical(lipgloss.Left, blocks[i], blocks[j]); got != want {
				t.Fatalf("vertical %d,%d\n got: %q\nwant: %q", i, j, got, want)
			}
			if got, want := joinHorizontalTop(blocks[i], blocks[j]), lipgloss.JoinHorizontal(lipgloss.Top, blocks[i], blocks[j]); got != want {
				t.Fatalf("horizontal %d,%d\n got: %q\nwant: %q", i, j, got, want)
			}
		}
	}
}
