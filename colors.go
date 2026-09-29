package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"
)

// The colour controller: a hidden editor reached from the help overlay (press
// ? then c). Every change is applied live — the whole UI, the visualizer
// included, draws from the same eight globals — so what you see is what you
// save. Enter keeps it, esc puts the old theme back.

// slotNames are the eight palette entries, in the order the editor lists them.
var slotNames = [8]string{"accent", "accent hi", "accent lo", "text", "text dim", "text faint", "border", "selection"}

func (t *theme) slot(i int) *lipgloss.Color {
	return [8]*lipgloss.Color{&t.accent, &t.accentHi, &t.accentLo, &t.fgBright, &t.fgDim, &t.fgFaint, &t.borderDim, &t.selBg}[i]
}

type colorEditor struct {
	open     bool
	row      int    // 0 = preset row, 1..8 = slots
	preset   int    // index into themes of the preset the draft started from
	draft    theme  // what is being edited (and is applied live)
	orig     theme  // restored on esc
	origName string // m.themeName on open
	edited   bool   // a slot was changed since the last preset pick
}

func (m *model) openColors() {
	cur := themes[0]
	if t := themeByName(m.themeName); t != nil {
		cur = *t
	}
	// Start from what is on screen, which for "auto" is the artwork's accent.
	cur.accent, cur.accentHi, cur.accentLo = accent, accentHi, accentLo
	idx := 0
	for i := range themes {
		if themes[i].name == m.themeName {
			idx = i
		}
	}
	m.colors = colorEditor{open: true, preset: idx, draft: cur, orig: cur, origName: m.themeName}
}

func (m model) updateColors(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	e := &m.colors
	switch msg.String() {
	case "esc", "q":
		applyTheme(e.orig)
		m.themeName = e.origName
		m.applyAutoTheme()
		e.open = false
		return m, nil
	case "enter":
		name := themes[e.preset].name
		if e.edited {
			e.draft.name = "custom"
			saveCustomTheme(e.draft)
			registerTheme(e.draft)
			name = "custom"
		}
		m.themeName = name
		saveThemeName(name)
		applyTheme(e.draft)
		m.applyAutoTheme()
		e.open = false
		m.note, m.noteAt = "theme · "+name, m.t
		return m, nil
	case "up", "k":
		e.row = (e.row + 8) % 9
	case "down", "j", "tab":
		e.row = (e.row + 1) % 9
	case "r":
		e.draft, e.edited = themes[e.preset], false
	case "left", "h", "right", "l":
		step := 1
		if s := msg.String(); s == "left" || s == "h" {
			step = -1
		}
		if e.row == 0 {
			e.preset = (e.preset + step + len(themes)) % len(themes)
			e.draft, e.edited = themes[e.preset], false
		} else {
			e.adjust(func(h, s, l float64) (float64, float64, float64) { return h + 6*float64(step), s, l })
		}
	case "[", "]":
		d := -0.04
		if msg.String() == "]" {
			d = 0.04
		}
		e.adjust(func(h, s, l float64) (float64, float64, float64) { return h, s, l + d })
	case "-", "=", "+":
		d := -0.05
		if msg.String() != "-" {
			d = 0.05
		}
		e.adjust(func(h, s, l float64) (float64, float64, float64) { return h, s + d, l })
	}
	applyTheme(e.draft)
	return m, nil
}

// adjust edits the selected slot in HSL space.
func (e *colorEditor) adjust(f func(h, s, l float64) (float64, float64, float64)) {
	if e.row < 1 {
		return
	}
	dst := e.draft.slot(e.row - 1)
	c, err := colorful.Hex(string(*dst))
	if err != nil {
		return
	}
	h, s, l := f(c.Hsl())
	h = math.Mod(h+360, 360)
	s = min(max(s, 0), 1)
	l = min(max(l, 0), 1)
	*dst = lipgloss.Color(colorful.Hsl(h, s, l).Clamped().Hex())
	e.edited = true
}

func (m model) colorsView() string {
	e := m.colors
	faint := lipgloss.NewStyle().Foreground(fgFaint)
	dim := lipgloss.NewStyle().Foreground(fgDim)
	bright := lipgloss.NewStyle().Foreground(fgBright)
	sel := lipgloss.NewStyle().Foreground(accentHi).Bold(true)

	label := func(row int, text string) string {
		if e.row == row {
			return sel.Render("▸ " + text)
		}
		return dim.Render("  " + text)
	}
	presetName := themes[e.preset].name
	if e.edited {
		presetName += faint.Render(" → custom")
	}
	lines := []string{
		bright.Bold(true).Render(" colours") + faint.Render("  (hidden: ? then c)"),
		"",
		label(0, fmt.Sprintf("%-11s", "theme")) + "  ‹ " + bright.Render(presetName) + " ›",
		"",
	}
	for i, name := range slotNames {
		c := *e.draft.slot(i)
		swatch := lipgloss.NewStyle().Background(c).Render("      ")
		lines = append(lines, label(i+1, fmt.Sprintf("%-11s", name))+"  "+swatch+" "+faint.Render(string(c)))
	}
	lines = append(lines, "",
		faint.Render(" ↑↓ pick  ←→ theme / hue  [ ] light  - = saturation"),
		faint.Render(" r reset  enter keep  esc cancel"))

	// A live visualizer strip, so the effect on the bars/orb is visible here.
	vw := min(46, max(20, m.w/3))
	preview := m.vizPanel(vw, 10)
	body := joinHorizontalTop(strings.Join(lines, "\n"), "   ", preview)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(1, 2).Render(body)
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, box)
}

// --- the custom palette on disk: one "slot = #rrggbb" line per colour -------

func customThemeFile() string {
	d := configDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "theme-custom")
}

var slotKeys = [8]string{"accent", "accent_hi", "accent_lo", "fg_bright", "fg_dim", "fg_faint", "border_dim", "sel_bg"}

func saveCustomTheme(t theme) {
	p := customThemeFile()
	if p == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	var b strings.Builder
	b.WriteString("# amtui custom palette, written by the colour editor (? then c)\n")
	for i, k := range slotKeys {
		fmt.Fprintf(&b, "%s = %q\n", k, string(*t.slot(i))) // quoted: a bare # starts a comment
	}
	_ = os.WriteFile(p, []byte(b.String()), 0o644)
}

// loadCustomTheme registers the saved custom palette, if there is one.
func loadCustomTheme() {
	p := customThemeFile()
	if p == "" {
		return
	}
	f, err := os.Open(p)
	if err != nil {
		return
	}
	defer f.Close()
	t := themes[0]
	t.name = "custom"
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(stripComment(sc.Text()), "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"`)
		for i, sk := range slotKeys {
			if k == sk {
				if _, err := colorful.Hex(v); err == nil {
					*t.slot(i) = lipgloss.Color(v)
				}
			}
		}
	}
	registerTheme(t)
}

// registerTheme adds or replaces a named theme.
func registerTheme(t theme) {
	if existing := themeByName(t.name); existing != nil {
		*existing = t
		return
	}
	themes = append(themes, t)
}
