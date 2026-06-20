package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// callSeen drives handleSeen and returns the decoded JSON body.
func callSeen(t *testing.T, s *Server, method, body string) map[string]any {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, "/api/seen", nil)
	} else {
		r = httptest.NewRequest(method, "/api/seen", bytes.NewReader([]byte(body)))
	}
	w := httptest.NewRecorder()
	s.handleSeen(w, r)
	if w.Code != 200 {
		t.Fatalf("%s /api/seen: status %d body %s", method, w.Code, w.Body.String())
	}
	var out map[string]any
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &out)
	}
	return out
}

// TestSeenSingleUpdateMonotonic: a read sets the channel's last-seen ID, a
// lower ID never moves it backwards (guards against races / out-of-order
// posts), and a higher ID advances it. Hash is overwritten as-is.
func TestSeenSingleUpdateMonotonic(t *testing.T) {
	s := newTestServerWithProfiles(t, &ProfileList{})
	callSeen(t, s, "POST", `{"name":"@chan","id":50,"hash":111}`)
	if got := loadProfilesT(t, s).SeenIDs["@chan"]; got != 50 {
		t.Fatalf("SeenIDs[@chan] = %d, want 50", got)
	}
	callSeen(t, s, "POST", `{"name":"@chan","id":10}`) // lower → ignored
	if got := loadProfilesT(t, s).SeenIDs["@chan"]; got != 50 {
		t.Errorf("SeenIDs[@chan] regressed to %d, want 50", got)
	}
	callSeen(t, s, "POST", `{"name":"@chan","id":75}`) // higher → advances
	pl := loadProfilesT(t, s)
	if pl.SeenIDs["@chan"] != 75 {
		t.Errorf("SeenIDs[@chan] = %d, want 75", pl.SeenIDs["@chan"])
	}
	if pl.SeenHashes["@chan"] != 111 {
		t.Errorf("SeenHashes[@chan] = %d, want 111", pl.SeenHashes["@chan"])
	}
}

// TestSeenBulkMigrationFillsOnlyGaps: the localStorage→disk migration must
// seed channels the server lacks but never clobber an existing marker.
func TestSeenBulkMigrationFillsOnlyGaps(t *testing.T) {
	s := newTestServerWithProfiles(t, &ProfileList{
		SeenIDs: map[string]int64{"@a": 100},
	})
	callSeen(t, s, "POST", `{"seenIds":{"@a":5,"@b":7}}`)
	pl := loadProfilesT(t, s)
	if pl.SeenIDs["@a"] != 100 {
		t.Errorf("@a clobbered to %d, want 100 (existing wins)", pl.SeenIDs["@a"])
	}
	if pl.SeenIDs["@b"] != 7 {
		t.Errorf("@b = %d, want 7 (gap filled)", pl.SeenIDs["@b"])
	}
}

// TestSeenSharedBackendIgnoresServer: in --shared mode, GET reports shared
// (and doesn't leak the stored maps) and POST is a no-op so connected users
// can't clobber a shared marker.
func TestSeenSharedBackendIgnoresServer(t *testing.T) {
	s := newTestServerWithProfiles(t, &ProfileList{SeenIDs: map[string]int64{"@a": 9}})
	s.sharedBackend = true
	out := callSeen(t, s, "GET", "")
	if out["shared"] != true {
		t.Errorf("GET shared = %v, want true", out["shared"])
	}
	if _, ok := out["seenIds"]; ok {
		t.Errorf("GET leaked seenIds in shared mode: %v", out)
	}
	callSeen(t, s, "POST", `{"name":"@a","id":500}`)
	if got := loadProfilesT(t, s).SeenIDs["@a"]; got != 9 {
		t.Errorf("shared POST mutated SeenIDs[@a] = %d, want 9 (no-op)", got)
	}
}

// TestSeenGetReturnsMaps round-trips the stored maps through GET.
func TestSeenGetReturnsMaps(t *testing.T) {
	s := newTestServerWithProfiles(t, &ProfileList{
		SeenIDs:    map[string]int64{"@a": 12},
		SeenHashes: map[string]int64{"@a": 34},
	})
	out := callSeen(t, s, "GET", "")
	ids, _ := out["seenIds"].(map[string]any)
	if ids == nil || ids["@a"] == nil {
		t.Fatalf("GET seenIds missing @a: %v", out)
	}
	if int64(ids["@a"].(float64)) != 12 {
		t.Errorf("seenIds[@a] = %v, want 12", ids["@a"])
	}
}
