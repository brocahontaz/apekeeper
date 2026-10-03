package logger

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type requestIDContextKey struct{}

// WithRequestID carries only the non-secret correlation identifier through
// backend work; it is safe to include in structured logs and metrics labels.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, id)
}
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

func New(level string) *slog.Logger {
	var l slog.Level
	_ = l.UnmarshalText([]byte(level))
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l, ReplaceAttr: redactAttr}))
}

var sensitiveKeys = map[string]bool{"access_token": true, "refresh_token": true, "client_secret": true, "authorization": true, "cookie": true, "set-cookie": true, "webhook_url": true, "webhook": true, "password": true, "secret": true}

func Redact(value string) string {
	var nested any
	if json.Unmarshal([]byte(value), &nested) == nil {
		if b, err := json.Marshal(redactJSON(nested)); err == nil {
			return string(b)
		}
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"access_token=", "refresh_token=", "authorization=", "cookie=", "webhook"} {
		if i := strings.Index(lower, marker); i >= 0 {
			return value[:i] + "[REDACTED]"
		}
	}
	if u, err := url.Parse(value); err == nil && strings.Contains(strings.ToLower(u.Path), "/webhook/") {
		return u.Scheme + "://" + u.Host + "/[REDACTED]"
	}
	return value
}
func redactJSON(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			key := strings.ToLower(k)
			if sensitiveKeys[key] || strings.Contains(key, "token") || strings.Contains(key, "secret") {
				x[k] = "[REDACTED]"
			} else {
				x[k] = redactJSON(value)
			}
		}
	case []any:
		for i := range x {
			x[i] = redactJSON(x[i])
		}
	}
	return v
}
func redactAttr(_ []string, a slog.Attr) slog.Attr {
	if sensitiveKeys[strings.ToLower(a.Key)] {
		return slog.String(a.Key, "[REDACTED]")
	}
	if a.Value.Kind() == slog.KindString {
		return slog.String(a.Key, Redact(a.Value.String()))
	}
	if a.Value.Kind() == slog.KindAny {
		return slog.Any(a.Key, redactJSON(a.Value.Any()))
	}
	return a
}

// statusWriter captures the response status code for request logging. A
// handler that writes a body without WriteHeader implicitly responds 200.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func RequestLog(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = RequestID(r.Context())
		}
		elapsed := time.Since(s)
		log.Info("request", "request_id", requestID, "method", r.Method, "path", r.URL.Path, "status", status, "duration", elapsed.String(), "duration_ms", elapsed.Milliseconds(), "error", status >= 400)
	})
}
