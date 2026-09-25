package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type testPinger struct{ err error }

func (p testPinger) Ping(context.Context) error { return p.err }
func TestHealth(t *testing.T) {
	for _, tc := range []struct {
		p    testPinger
		code int
	}{{testPinger{}, 200}, {testPinger{errors.New("down")}, 503}} {
		r := httptest.NewRecorder()
		health(tc.p).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if r.Code != tc.code {
			t.Fatalf("code %d", r.Code)
		}
	}
}
