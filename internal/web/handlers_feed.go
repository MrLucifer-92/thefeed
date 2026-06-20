package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	mrand "math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sartoopjj/thefeed/internal/client"
	"github.com/sartoopjj/thefeed/internal/protocol"
	"github.com/sartoopjj/thefeed/internal/version"
)

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]any{
		"configured":  s.config != nil,
		"version":     version.Version,
		"hasPassword": s.password != "",
	}
	if s.config != nil {
		status["domain"] = s.config.Domain
		status["channels"] = s.channels
		status["telegramLoggedIn"] = s.telegramLoggedIn
		status["nextFetch"] = s.nextFetch
		status["latestVersion"] = s.latestVersion
		// Include last resolver scan if recent (<24 h) so the frontend can offer a quick-start.
		if ls := s.loadLastScan(); ls != nil {
			status["lastScan"] = map[string]any{
				"resolvers": ls.Resolvers,
				"scannedAt": ls.ScannedAt,
				"count":     len(ls.Resolvers),
			}
		}
	}
	writeJSON(w, status)
}

// handleConfig handles GET (read) and POST (write) of client configuration.
// POST is authenticated when a global password is set (via the middleware).
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		defer s.mu.RUnlock()
		if s.config == nil {
			writeJSON(w, map[string]any{"configured": false})
			return
		}
		writeJSON(w, s.config)

	case http.MethodPost:
		var cfg Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		if cfg.Domain == "" || cfg.Key == "" || len(cfg.Resolvers) == 0 {
			http.Error(w, "domain, key, and resolvers are required", 400)
			return
		}
		// Add config resolvers to the shared bank.
		if len(cfg.Resolvers) > 0 {
			pl, _ := s.loadProfiles()
			if pl == nil {
				pl = &ProfileList{}
			}
			addToBank(pl, cfg.Resolvers)
			_ = s.saveProfiles(pl)
		}
		if err := s.saveConfig(&cfg); err != nil {
			http.Error(w, fmt.Sprintf("save config: %v", err), 500)
			return
		}
		s.mu.Lock()
		s.config = &cfg
		s.mu.Unlock()

		if err := s.initFetcher(); err != nil {
			http.Error(w, fmt.Sprintf("init fetcher: %v", err), 500)
			return
		}
		s.startCheckerThenRefresh()
		writeJSON(w, map[string]any{"ok": true})

	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	writeJSON(w, s.channels)
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "missing channel number", 400)
		return
	}
	chNum, err := strconv.Atoi(parts[3])
	if err != nil || chNum < 1 {
		http.Error(w, "invalid channel number", 400)
		return
	}

	s.mu.RLock()
	msgs := s.messages[chNum]
	chs := s.channels
	cache := s.cache
	s.mu.RUnlock()

	// Serve the persistent on-disk cache when available —
	// it contains the full merged history (up to 200 messages) keyed by channel name.
	if cache != nil && chNum >= 1 && chNum <= len(chs) {
		if result := cache.GetMessages(chs[chNum-1].Name); result != nil {
			writeJSON(w, result)
			return
		}
	}

	// Fall back to the in-memory fresh fetch (no accumulated history).
	writeJSON(w, client.NewMessagesResult(msgs))
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	// Background (quiet) metadata-only refreshes skip silently if one is already running,
	// so the auto-refresh timer never cancels a slow in-progress fetch.
	// Channel refreshes are NOT skipped here — refreshChannel has its own duplicate guard.
	chParam := r.URL.Query().Get("channel")
	if r.URL.Query().Get("quiet") == "1" && chParam == "" {
		s.refreshMu.Lock()
		running := len(s.refreshCancels) > 0
		s.refreshMu.Unlock()
		if running {
			writeJSON(w, map[string]any{"ok": true, "skipped": true})
			return
		}
	}
	if chParam != "" {
		chNum, err := strconv.Atoi(chParam)
		if err != nil || chNum < 1 {
			http.Error(w, "invalid channel", 400)
			return
		}
		go s.refreshChannel(chNum)
	} else {
		go s.refreshMetadataOnly()
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleRescan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	s.mu.RLock()
	checker := s.checker
	fetcher := s.fetcher
	baseCtx := s.fetcherCtx
	s.mu.RUnlock()
	if checker == nil || baseCtx == nil {
		http.Error(w, "not configured", 400)
		return
	}
	// Widen the fetcher's pool to the full bank so we probe every
	// known resolver, not just the currently-active subset.
	// rescanReplaceList tells SetOnScanDone to overwrite the list.
	pl, _ := s.loadProfiles()
	if pl != nil && len(pl.ResolverBank) > 0 && fetcher != nil {
		fetcher.UpdateResolverPool(pl.ResolverBank)
	}
	s.rescanFlagMu.Lock()
	s.rescanReplaceList = true
	s.rescanFlagMu.Unlock()

	go func() {
		// Cancel any in-progress metadata refresh so it doesn't race with the
		// scan — we want fresh resolver data before we hit DNS again.
		s.refreshMu.Lock()
		for k, cancel := range s.refreshCancels {
			cancel()
			delete(s.refreshCancels, k)
		}
		s.refreshMu.Unlock()

		if checker.CheckNow(baseCtx) {
			// Cool-down: give resolvers time to recover from the scan's DNS
			// queries before we immediately hit them again with a fetch.
			sleep := 3*time.Second + time.Duration(mrand.IntN(13))*time.Second // 3–15 s
			select {
			case <-baseCtx.Done():
				return
			case <-time.After(sleep):
			}
			s.refreshMetadataOnly()
		}
	}()
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}

	var req struct {
		Channel int    `json:"channel"`
		Text    string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if req.Channel < 1 || req.Text == "" {
		http.Error(w, "channel and text are required", 400)
		return
	}
	if len(req.Text) > 4000 {
		http.Error(w, "message too long (max 4000 chars)", 400)
		return
	}

	s.mu.RLock()
	fetcher := s.fetcher
	basectx := s.fetcherCtx
	s.mu.RUnlock()

	if fetcher == nil || basectx == nil {
		http.Error(w, "not configured", 400)
		return
	}

	ctx, cancel := context.WithTimeout(basectx, 5*time.Minute)
	defer cancel()

	s.addLog(fmt.Sprintf("Sending message to channel %d (%d chars)...", req.Channel, len(req.Text)))

	if err := fetcher.SendMessage(ctx, req.Channel, req.Text); err != nil {
		log.Printf("[web] send error ch=%d: %v", req.Channel, err)
		s.addLog("Error: failed to send message")
		http.Error(w, "failed to send message", 500)
		return
	}

	s.addLog("Message sent successfully")
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}

	var req struct {
		Command string `json:"command"`
		Arg     string `json:"arg"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if req.Command == "" {
		http.Error(w, "command is required", 400)
		return
	}

	s.mu.RLock()
	fetcher := s.fetcher
	basectx := s.fetcherCtx
	s.mu.RUnlock()

	if fetcher == nil || basectx == nil {
		http.Error(w, "not configured", 400)
		return
	}

	ctx, cancel := context.WithTimeout(basectx, 5*time.Minute)
	defer cancel()

	s.addLog(fmt.Sprintf("Admin command: %s %s", req.Command, req.Arg))

	var cmd protocol.AdminCmd
	switch req.Command {
	case "add_channel":
		cmd = protocol.AdminCmdAddChannel
	case "remove_channel":
		cmd = protocol.AdminCmdRemoveChannel
	case "list_channels":
		cmd = protocol.AdminCmdListChannels
	case "refresh":
		cmd = protocol.AdminCmdRefresh
	default:
		http.Error(w, "unknown command", 400)
		return
	}

	result, err := fetcher.SendAdminCommand(ctx, cmd, req.Arg)
	if err != nil {
		log.Printf("[web] admin error: %v", err)
		s.addLog(fmt.Sprintf("Admin error: %v", err))
		http.Error(w, "admin command failed", 500)
		return
	}

	s.addLog(fmt.Sprintf("Admin result: %s", result))
	writeJSON(w, map[string]any{"ok": true, "result": result})
}
