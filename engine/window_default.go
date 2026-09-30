//go:build !darwin && !windows

package engine

// X11 and friends let a normal window sit fully offscreen and keep rendering,
// so the CDP park/minimize pair works as written.
func defaultWindowController(int) windowController { return cdpWindowController{} }
