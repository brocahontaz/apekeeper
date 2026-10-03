package httpapi

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestRouteManifestMatchesOpenAPIRoutes(t *testing.T) {
	b, err := os.ReadFile("../../../openapi/routes.json")
	if err != nil {
		t.Fatal(err)
	}
	var documented map[string]struct {
		Roles []string `json:"roles"`
	}
	if err := json.Unmarshal(b, &documented); err != nil {
		t.Fatal(err)
	}
	actual := make(map[string]struct {
		Roles []string `json:"roles"`
	}, len(RouteManifest()))
	for _, route := range RouteManifest() {
		if _, duplicate := actual[route.Pattern]; duplicate {
			t.Fatalf("duplicate registered route %q", route.Pattern)
		}
		roles := route.Roles
		if roles == nil {
			roles = []string{}
		}
		actual[route.Pattern] = struct {
			Roles []string `json:"roles"`
		}{Roles: roles}
	}
	if !reflect.DeepEqual(actual, documented) {
		t.Fatalf("route manifest drift: registered=%v documented=%v", actual, documented)
	}
}
