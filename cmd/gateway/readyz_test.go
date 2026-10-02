package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type fakeZitiReadiness struct {
	established atomic.Uint32
}

func (f *fakeZitiReadiness) EstablishedListeners() uint {
	return uint(f.established.Load())
}

func assertReadyzStatus(t *testing.T, handler http.Handler, want int) {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, readyzPath, nil))
	if recorder.Code != want {
		t.Fatalf("expected status %d, got %d (%q)", want, recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("expected no-store cache control, got %q", got)
	}
}

func TestReadyzFollowsEstablishedZitiListeners(t *testing.T) {
	probe := &fakeZitiReadiness{}
	handler := newReadyzHandler(probe)

	assertReadyzStatus(t, handler, http.StatusServiceUnavailable)

	probe.established.Store(1)
	assertReadyzStatus(t, handler, http.StatusOK)

	probe.established.Store(0)
	assertReadyzStatus(t, handler, http.StatusServiceUnavailable)
}

func TestReadyzReadyWhenZitiDisabled(t *testing.T) {
	assertReadyzStatus(t, newReadyzHandler(nil), http.StatusOK)
}
