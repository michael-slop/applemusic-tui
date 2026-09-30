//go:build !windows

package engine

// chromedp's Pdeathsig already ends the browser with amtui on Linux; see
// lifetime_windows.go for why Windows needs these.

func tieBrowserToProcess(int) {}

func clearStaleBrowser(string) error { return nil }
