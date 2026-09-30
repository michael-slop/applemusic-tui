package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	colorful "github.com/lucasb-eyer/go-colorful"

	tea "github.com/charmbracelet/bubbletea"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func press(t *testing.T, m model, keys ...string) model {
	t.Helper()
	for _, k := range keys {
		next, _ := m.Update(key(k))
		m = next.(model)
	}
	return m
}

func TestColourControllerHidesBehindHelp(t *testing.T) {
	t.Setenv("AMTUI_CONFIG_DIR", t.TempDir())
	applyTheme(themes[0])
	m := model{w: 120, h: 35, phase: phaseReady, st: demoState(), themeName: "apple"}
	m = press(t, m, "?")
	if !m.helpOpen {
		t.Fatal("? should open help")
	}
	if strings.Contains(m.View(), "colours") {
		t.Fatal("the help overlay must not advertise the colour controller")
	}
	m = press(t, m, "c")
	if !m.colors.open || !strings.Contains(m.View(), "colours") {
		t.Fatal("c inside help should open the colour controller")
	}
}

func TestColourControllerEditsLiveAndEscRestores(t *testing.T) {
	t.Setenv("AMTUI_CONFIG_DIR", t.TempDir())
	applyTheme(themes[0])
	m := model{w: 120, h: 35, phase: phaseReady, st: demoState(), themeName: "apple"}
	before := accent
	m = press(t, m, "?", "c", "down", "right", "right")
	if accent == before {
		t.Fatal("changing the accent hue should apply live")
	}
	m = press(t, m, "esc")
	if accent != before || m.colors.open {
		t.Fatalf("esc should restore %s, got %s", before, accent)
	}
}

func TestColourControllerSavesCustomAndReloads(t *testing.T) {
	t.Setenv("AMTUI_CONFIG_DIR", t.TempDir())
	applyTheme(themes[0])
	m := model{w: 120, h: 35, phase: phaseReady, st: demoState(), themeName: "apple"}
	m = press(t, m, "?", "c", "down", "]", "]", "enter")
	if m.themeName != "custom" || loadThemeName() != "custom" {
		t.Fatalf("an edited palette should save as custom, got %q / %q", m.themeName, loadThemeName())
	}
	saved := accent
	// Simulate a restart: drop the in-memory custom theme and reload it.
	for i := range themes {
		if themes[i].name == "custom" {
			themes = append(themes[:i], themes[i+1:]...)
			break
		}
	}
	loadCustomTheme()
	if c := themeByName("custom"); c == nil || c.accent != saved {
		t.Fatalf("custom palette did not survive a reload: %+v", c)
	}
}

func TestPresetRowSelectsTheHousePalette(t *testing.T) {
	t.Setenv("AMTUI_CONFIG_DIR", t.TempDir())
	applyTheme(themes[0])
	m := model{w: 120, h: 35, phase: phaseReady, st: demoState(), themeName: "apple"}
	m = press(t, m, "?", "c")
	for i := 0; i < len(themes) && themes[m.colors.preset].name != "slop"; i++ {
		m = press(t, m, "right")
	}
	m = press(t, m, "enter")
	if m.themeName != "slop" || accent != "#62E670" {
		t.Fatalf("expected the slop house theme, got %q accent %s", m.themeName, accent)
	}
}

// Every preset must be usable: valid colours, a unique name, and text that
// reads against the selection background.
func TestEveryThemePresetIsUsable(t *testing.T) {
	checkThemesUsable(t)
	if len(themes) < 18 {
		t.Fatalf("expected the IDE presets to be registered, have %d themes", len(themes))
	}
}

// checkThemesUsable: valid colours, unique names, text that reads against the
// selection background (either way round: light themes are dark-on-light).
func checkThemesUsable(t *testing.T) {
	t.Helper()
	seen := map[string]bool{}
	for _, th := range themes {
		if seen[th.name] {
			t.Fatalf("duplicate theme name %q", th.name)
		}
		seen[th.name] = true
		for i := range 8 {
			if _, err := colorful.Hex(string(*th.slot(i))); err != nil {
				t.Fatalf("%s: slot %s is not a colour: %q", th.name, slotNames[i], *th.slot(i))
			}
		}
		text, _ := colorful.Hex(string(th.fgBright))
		sel, _ := colorful.Hex(string(th.selBg))
		if _, _, lt := text.Hcl(); func() bool { _, _, ls := sel.Hcl(); return math.Abs(lt-ls) < 0.35 }() {
			t.Errorf("%s: text %s does not stand out from the selection %s", th.name, th.fgBright, th.selBg)
		}
	}
}

// The menu shared with color.mesh: built from ~/.config/color.mesh/presets.conf
// and the Omarchy themes' colors.toml, keeping amtui's own apple/mono/auto,
// and every preset it produces must be usable (checked above for all themes).
func TestMeshPresetsReplaceTheIDEBlock(t *testing.T) {
	saved := append([]theme(nil), themes...)
	defer func() { themes = saved }()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OMARCHY_PATH", filepath.Join(home, "omarchy"))
	write := func(p, s string) {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(s), 0o644)
	}
	write(filepath.Join(home, ".config/color.mesh/presets.conf"),
		"# shared\nhouse | slop | slop (house)\nIDE | dracula | Dracula\nlight | white | White\nIDE | missing | Not Installed\n")
	write(filepath.Join(home, ".config/omarchy/themes/dracula/colors.toml"),
		"accent = \"#bd93f9\"\nbackground = \"#282a36\"\nforeground = \"#f8f8f2\"\ncolor8 = \"#6272a4\"\n")
	write(filepath.Join(home, "omarchy/themes/white/colors.toml"),
		"accent = \"#6e6e6e\"\nbackground = \"#ffffff\"\nforeground = \"#000000\"\nselection_background = \"#000000\"\n")
	if !loadMeshPresets() {
		t.Fatal("shared presets should load")
	}
	var names []string
	for _, th := range themes {
		names = append(names, th.name)
	}
	if got := strings.Join(names, ","); got != "apple,slop,dracula,white,mono,auto" {
		t.Fatalf("menu = %s", got)
	}
	if d := themeByName("dracula"); d.accent != "#bd93f9" || d.fgFaint != "#6272a4" {
		t.Fatalf("dracula mapped as %+v", *d)
	}
	if w := themeByName("white"); w.selBg == "#000000" {
		t.Fatal("selection must be a raised background, never an inverse-video foreground")
	}
	checkThemesUsable(t)
}
