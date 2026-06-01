package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sartoopjj/thefeed/internal/protocol"
)

// runMediaCacheSweep evicts expired media-cache entries every hour for the
// lifetime of the process.
func (s *Server) runMediaCacheSweep() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		if s.mediaCache == nil {
			return
		}
		s.mediaCache.Cleanup()
	}
}

// handleClearCache wipes both the per-channel message cache and the
// downloaded-media disk cache.
func (s *Server) handleClearCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	deleted := 0
	cacheDir := filepath.Join(s.dataDir, "cache")
	if entries, err := os.ReadDir(cacheDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if os.Remove(filepath.Join(cacheDir, e.Name())) == nil {
				deleted++
			}
		}
	}
	if s.telemirror != nil {
		s.telemirror.ClearCache()
	}
	if s.profilePics != nil {
		s.profilePics.Clear()
	}
	mediaDeleted := 0
	if s.mediaCache != nil {
		mediaDeleted = s.mediaCache.Clear()
	}
	_ = os.Remove(s.channelsCachePath())
	// Reset in-memory message state too so refreshChannel's "no changes"
	// guard doesn't skip the next fetch (prev IDs match what's on the
	// server, but our cache is gone).
	s.mu.Lock()
	s.messages = make(map[int][]protocol.Message)
	s.lastMsgIDs = make(map[int]uint32)
	s.lastHashes = make(map[int]uint32)
	s.mu.Unlock()
	s.addLog(fmt.Sprintf("Cache cleared: %d message files, %d media files", deleted, mediaDeleted))
	writeJSON(w, map[string]any{"ok": true, "deleted": deleted, "mediaDeleted": mediaDeleted})
}
