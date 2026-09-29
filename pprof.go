package main

import (
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof handlers on the default mux
	"os"
)

// AMTUI_PPROF=127.0.0.1:6060 exposes Go's profiler for performance work.
// Off by default; loopback only is the caller's responsibility.
func init() {
	if addr := os.Getenv("AMTUI_PPROF"); addr != "" {
		go func() { _ = http.ListenAndServe(addr, nil) }()
	}
}
