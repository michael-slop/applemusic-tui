package main

import (
	"strings"
	"testing"

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
