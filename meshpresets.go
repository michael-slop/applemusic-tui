package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"
)

// Shared presets with color.mesh (the desktop theme picker). Michael: "the
// only thing i want to sync is the selection of color presets" -- so amtui
// offers the SAME menu as color.mesh, read from the same file, while picking
// a theme here still only recolours amtui.
//
// ~/.config/color.mesh/presets.conf lists "group | id | name"; each id is an
// Omarchy theme whose colors.toml supplies the palette. Where the file or the
// themes do not exist (Windows, macOS, another Linux), the built-in presets in
// theme.go stand as they are.

// meshPresetsFile is the shared list.
func meshPresetsFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "color.mesh", "presets.conf")
}

// omarchyThemeDirs are where an id's colors.toml may live, user overlay last
// (it wins, as in omarchy-theme-set).
func omarchyThemeDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	root := os.Getenv("OMARCHY_PATH")
	if root == "" {
		root = filepath.Join(home, ".local", "share", "omarchy")
	}
	return []string{filepath.Join(root, "themes"), filepath.Join(home, ".config", "omarchy", "themes")}
}

// readColorsTOML reads the flat `key = "#rrggbb"` lines of a colors.toml.
func readColorsTOML(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, `"#`) || strings.HasPrefix(v, `'#`) {
			v = strings.Trim(strings.Fields(v)[0], `"'`)
		}
		if _, err := colorful.Hex(v); err == nil && !strings.HasPrefix(k, "#") {
			out[k] = strings.ToLower(v)
		}
	}
	return out
}

// themeFromOmarchy maps an Omarchy palette onto amtui's eight slots.
func themeFromOmarchy(id string, c map[string]string) (theme, bool) {
	get := func(k string) (colorful.Color, bool) {
		col, err := colorful.Hex(c[k])
		return col, err == nil && c[k] != ""
	}
	bg, ok1 := get("background")
	fg, ok2 := get("foreground")
	acc, ok3 := get("accent")
	if !ok1 || !ok2 || !ok3 {
		return theme{}, false
	}
	hex := func(x colorful.Color) lipgloss.Color { return lipgloss.Color(x.Clamped().Hex()) }
	faint := fg.BlendRgb(bg, 0.55)
	if c8, ok := get("color8"); ok && c8.DistanceLab(bg) > 0.15 {
		faint = c8 // the theme's own comment grey, when it reads against the background
	}
	// Selection is a raised background, never Omarchy's selection_background:
	// many themes set that to the foreground (inverse video), which would put
	// amtui's selected-row text on its own colour.
	sel := bg.BlendRgb(fg, 0.14)
	if lb, ok := get("lighter_background"); ok && lb.DistanceLab(bg) > 0.03 && lb.DistanceLab(fg) > 0.3 {
		sel = lb
	}
	return theme{
		name:      id,
		accent:    hex(acc),
		accentHi:  hex(acc.BlendRgb(fg, 0.35)),
		accentLo:  hex(acc.BlendRgb(bg, 0.45)),
		fgBright:  hex(fg),
		fgDim:     hex(fg.BlendRgb(bg, 0.30)),
		fgFaint:   hex(faint),
		borderDim: hex(bg.BlendRgb(fg, 0.22)),
		selBg:     hex(sel),
	}, true
}

// legacyThemeNames maps names amtui used before the shared list to their
// shared ids, so a saved choice survives the switch.
var legacyThemeNames = map[string]string{"monokai-pro": "monokai", "solarized": "solarized-dark"}

// loadMeshPresets replaces the built-in IDE presets with the shared list,
// keeping amtui's own apple, mono and auto. It returns false (and changes
// nothing) when the shared list is absent or yields no installed theme.
func loadMeshPresets() bool {
	p := meshPresetsFile()
	if p == "" {
		return false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	dirs := omarchyThemeDirs()
	var shared []theme
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			continue
		}
		id := strings.TrimSpace(parts[1])
		if id == "slop" {
			// The house palette keeps amtui's hand-tuned slots (the same
			// palette; hand-mapped rather than derived).
			if t := themeByName("slop"); t != nil {
				shared = append(shared, *t)
			}
			continue
		}
		colors := map[string]string{}
		for _, d := range dirs {
			for k, v := range readColorsTOML(filepath.Join(d, id, "colors.toml")) {
				colors[k] = v
			}
		}
		if t, ok := themeFromOmarchy(id, colors); ok {
			shared = append(shared, t)
		}
	}
	if len(shared) == 0 {
		return false
	}
	keep := func(name string) theme { return *themeByName(name) }
	next := []theme{keep("apple")}
	next = append(next, shared...)
	next = append(next, keep("mono"), keep("auto"))
	themes = next
	return true
}
