package web

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// SetSharedBackend toggles multi-user mode. When on, /api/seen reports
// shared:true and refuses writes, so the client falls back to per-browser
// localStorage and connected users don't share (and clobber) seen markers.
func (s *Server) SetSharedBackend(v bool) { s.sharedBackend = v }

// handleSeen persists per-channel read markers (last-seen message ID and
// content hash) so the unread-count badges survive the client's loopback
// port changing — the markers used to live only in WebView localStorage,
// which is wiped when the origin (port) changes between launches.
//
// GET returns the stored maps. POST either merges full baseline maps (to
// seed channels the server doesn't track yet) or applies a single channel
// update on read. Single-channel IDs only ever move forward; bulk merges
// only fill gaps, so a real read marker is never overwritten by a baseline.
func (s *Server) handleSeen(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.sharedBackend {
			// Multi-user backend: tell the client to use localStorage only.
			writeJSON(w, map[string]any{"shared": true})
			return
		}
		pl, _ := s.loadProfiles()
		if pl == nil {
			pl = &ProfileList{}
		}
		writeJSON(w, map[string]any{
			"seenIds":    pl.SeenIDs,
			"seenHashes": pl.SeenHashes,
		})

	case http.MethodPost:
		if s.sharedBackend {
			// Refuse writes in multi-user mode so one user can't persist a
			// shared marker; the client keeps its own state in localStorage.
			writeJSON(w, map[string]any{"ok": true, "shared": true})
			return
		}
		var req struct {
			Name string `json:"name"`
			ID   *int64 `json:"id"`
			Hash *int64 `json:"hash"`
			// Bulk maps: the client's full baseline maps, sent to seed
			// channels the server doesn't track yet (new channels / first
			// load against an empty store). NOT a localStorage migration —
			// the client never uploads its old localStorage; it wipes it.
			SeenIDs    map[string]int64 `json:"seenIds"`
			SeenHashes map[string]int64 `json:"seenHashes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		s.profilesMu.Lock()
		defer s.profilesMu.Unlock()
		pl, err := s.loadProfilesExisting()
		if err != nil {
			http.Error(w, fmt.Sprintf("load: %v", err), 500)
			return
		}
		if pl == nil {
			pl = &ProfileList{}
		}
		if pl.SeenIDs == nil {
			pl.SeenIDs = map[string]int64{}
		}
		if pl.SeenHashes == nil {
			pl.SeenHashes = map[string]int64{}
		}

		// Baseline seed: only fill entries the server doesn't already have,
		// so an existing read marker is never clobbered by a baseline.
		for k, v := range req.SeenIDs {
			if _, ok := pl.SeenIDs[k]; !ok {
				pl.SeenIDs[k] = v
			}
		}
		for k, v := range req.SeenHashes {
			if _, ok := pl.SeenHashes[k]; !ok {
				pl.SeenHashes[k] = v
			}
		}

		// Single-channel read update. IDs are monotonic (seen-up-to), so
		// never let a race move the marker backwards.
		if req.Name != "" {
			if req.ID != nil && *req.ID > pl.SeenIDs[req.Name] {
				pl.SeenIDs[req.Name] = *req.ID
			}
			if req.Hash != nil {
				pl.SeenHashes[req.Name] = *req.Hash
			}
		}

		if err := s.saveProfiles(pl); err != nil {
			http.Error(w, fmt.Sprintf("save: %v", err), 500)
			return
		}
		writeJSON(w, map[string]any{"ok": true})

	default:
		http.Error(w, "method not allowed", 405)
	}
}
