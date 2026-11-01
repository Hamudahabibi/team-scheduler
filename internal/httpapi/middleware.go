package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

type ctxKey int

const (
	ctxKeyLogger ctxKey = iota
	ctxKeyUser
)

func loggerFrom(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKeyLogger).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// statusRecorder запоминает код ответа для логирования.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// withRequestLog присваивает запросу id, кладёт логгер с этим id в контекст
// и пишет строку лога по завершении запроса.
func withRequestLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = newRequestID()
		}
		w.Header().Set("X-Request-ID", reqID)

		l := log.With(slog.String("request_id", reqID))
		ctx := context.WithValue(r.Context(), ctxKeyLogger, l)
		rec := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(rec, r.WithContext(ctx))

		l.Info("http",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Int("bytes", rec.bytes),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

// withRecover превращает панику в обработчике в ответ 500.
func withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				loggerFrom(r.Context()).Error("паника в обработчике",
					slog.Any("panic", v), slog.String("stack", string(debug.Stack())))
				writeJSON(w, http.StatusInternalServerError, errorBody{errorDetail{
					Code: "internal", Message: "внутренняя ошибка сервера",
				}})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
