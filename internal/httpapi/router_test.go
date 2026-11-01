package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestHealthAndReady(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		dbErr  error
		status int
	}{
		{"healthz", "/healthz", nil, http.StatusOK},
		{"readyz ok", "/readyz", nil, http.StatusOK},
		{"readyz без БД", "/readyz", errors.New("нет соединения"), http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewRouter(Deps{Log: testLogger(), DB: fakePinger{tt.dbErr}})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.status {
				t.Fatalf("код %d, ожидался %d", rec.Code, tt.status)
			}
			if rec.Header().Get("X-Request-ID") == "" {
				t.Error("нет заголовка X-Request-ID")
			}
		})
	}
}
