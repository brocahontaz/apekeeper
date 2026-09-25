package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestLogRecordsResponseStatus(t *testing.T) {
	for name, test := range map[string]struct {
		handler http.HandlerFunc
		want    int
	}{
		"explicit 201": {func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}, http.StatusCreated},
		"implicit 500 via http.Error": {func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}, http.StatusInternalServerError},
		"handler writes nothing": {func(http.ResponseWriter, *http.Request) {}, http.StatusOK},
	} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			RequestLog(test.handler, slog.New(slog.NewJSONHandler(&buf, nil))).
				ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test", nil))

			var entry map[string]any
			if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
				t.Fatalf("request log entry = %q: %v", buf.String(), err)
			}
			if entry["msg"] != "request" {
				t.Errorf("msg = %v, want request", entry["msg"])
			}
			if status, ok := entry["status"].(float64); !ok || int(status) != test.want {
				t.Errorf("status = %v, want %d", entry["status"], test.want)
			}
			if _, ok := entry["duration"]; !ok {
				t.Error("request log entry missing duration")
			}
		})
	}
}
