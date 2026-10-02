package main

import (
	"io"
	"net/http"
)

const readyzPath = "/readyz"

// zitiReadiness reports router-confirmed terminators of the Ziti service listener.
type zitiReadiness interface {
	EstablishedListeners() uint
}

// newReadyzHandler serves unauthenticated Ziti-listener readiness: ready when
// Ziti is disabled (nil probe) or at least one terminator is established. It
// must not gate the TCP Service: the TCP API does not need Ziti, and a router
// restart would remove every replica at once. Liveness must not use it either:
// binding can legitimately take up to ZITI_BIND_TIMEOUT, and the TCP server
// starts before enrollment.
func newReadyzHandler(probe zitiReadiness) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if probe != nil && probe.EstablishedListeners() == 0 {
			http.Error(w, "ziti service listener not established", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok\n")
	})
}
