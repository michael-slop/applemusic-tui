//go:build !linux && !windows

package mpris

import "fmt"

// MPRIS is a freedesktop thing. macOS gets a no-op with the same shape so the
// caller needs no build tags of its own (Windows has media keys while the
// browser sleeps: mediakeys_windows.go). State and Controls are
// shared — only the D-Bus server is Linux-only.

type Server struct{}

func Publish(Controls) (*Server, error) {
	return nil, fmt.Errorf("mpris: only available on Linux")
}

func (s *Server) Update(State) {}
func (s *Server) Close()       {}
