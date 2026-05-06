package spec

import (
	"testing"
)

// TestEveryExtractedRouteHasResponse is a sanity check that the generator
// populated meaningful payloads for every extracted route. A route with no
// response field usually means the response key wasn't 200/2xx, which is
// worth investigating.
func TestEveryExtractedRouteHasResponse(t *testing.T) {
	for key, route := range All() {
		if len(route.Response) == 0 {
			t.Errorf("route %q has no response schema — generator may be misconfigured", key)
		}
		if route.Path == "" || route.Method == "" {
			t.Errorf("route %q has empty Path or Method", key)
		}
	}
}

func TestLookup_Hit(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Skip("no schemas extracted; nothing to look up")
	}
	for key := range all {
		_, ok := Lookup(key)
		if !ok {
			t.Errorf("Lookup(%q) returned not-ok despite key being in All()", key)
		}
		break
	}
}

func TestLookup_Miss(t *testing.T) {
	if _, ok := Lookup("GET /no/such/route"); ok {
		t.Error("Lookup of unknown key should return ok=false")
	}
}
