package api

import (
	"net/http"
	"net/http/pprof"
)

// RegisterPprof mounts Go's built-in profiler endpoints onto our own
// mux instead of relying on http.DefaultServeMux (which pprof's blank
// import registers to by default, but we run a custom mux so we mount
// these explicitly to actually get them on the port that matters).
func RegisterPprof(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}
