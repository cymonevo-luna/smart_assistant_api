package middleware

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cymonevo/go_template/pkg/logger"
)

// hijackableWriter is a ResponseWriter that also supports hijacking, mimicking
// the writer net/http hands to the server (which WebSocket upgrades rely on).
type hijackableWriter struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (w *hijackableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijacked = true
	return nil, nil, nil
}

func newTestLogger(t *testing.T) logger.Logger {
	t.Helper()
	log, err := logger.New("error", false)
	if err != nil {
		t.Fatalf("logger.New: %v", err)
	}
	return log
}

// TestLoggerPreservesHijacker guards the WebSocket upgrade path: the logging
// middleware wraps the ResponseWriter, and that wrapper must forward Hijack or
// gorilla/websocket upgrades fail with "http.Hijacker is unavailable".
func TestLoggerPreservesHijacker(t *testing.T) {
	var (
		sawHijacker bool
		hijackErr   error
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		sawHijacker = ok
		if ok {
			_, _, hijackErr = hj.Hijack()
		}
	})

	wrapped := Logger(newTestLogger(t))(handler)

	rw := &hijackableWriter{ResponseRecorder: httptest.NewRecorder()}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/stream", nil)
	wrapped.ServeHTTP(rw, req)

	if !sawHijacker {
		t.Fatal("ResponseWriter passed through Logger does not implement http.Hijacker")
	}
	if hijackErr != nil {
		t.Fatalf("Hijack returned error: %v", hijackErr)
	}
	if !rw.hijacked {
		t.Fatal("Hijack was not forwarded to the underlying ResponseWriter")
	}
}

// TestLoggerHijackWithoutSupport ensures a clear error (not a panic) when the
// underlying writer cannot be hijacked.
func TestLoggerHijackWithoutSupport(t *testing.T) {
	var hijackErr error

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("expected wrapper to advertise http.Hijacker")
		}
		_, _, hijackErr = hj.Hijack()
	})

	wrapped := Logger(newTestLogger(t))(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	wrapped.ServeHTTP(httptest.NewRecorder(), req)

	if hijackErr == nil {
		t.Fatal("expected an error when underlying writer is not a Hijacker")
	}
}
