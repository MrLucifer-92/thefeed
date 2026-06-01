package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sartoopjj/thefeed/internal/protocol"
)

func (s *Server) loadConfig() (*Config, error) {
	path := filepath.Join(s.dataDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// saveLastScan persists the healthy resolver list from the most recent scan.
func (s *Server) saveLastScan(resolvers []string) {
	d := lastScanData{Resolvers: resolvers, ScannedAt: time.Now().Unix()}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(s.dataDir, "last_scan.json"), b, 0600)
}

func (s *Server) activeProfileID() string {
	pl, err := s.loadProfiles()
	if err != nil || pl == nil {
		return ""
	}
	return pl.Active
}

func (s *Server) channelsCachePath() string {
	return filepath.Join(s.dataDir, "channels_cache.json")
}

func (s *Server) readChannelsCacheFile() channelsCacheFile {
	b, err := os.ReadFile(s.channelsCachePath())
	if err != nil {
		return channelsCacheFile{}
	}
	var f channelsCacheFile
	if err := json.Unmarshal(b, &f); err != nil || f == nil {
		return channelsCacheFile{}
	}
	return f
}

func (s *Server) saveChannelsCache(channels []protocol.ChannelInfo, nextFetch uint32) {
	id := s.activeProfileID()
	if id == "" || len(channels) == 0 {
		return
	}
	f := s.readChannelsCacheFile()
	f[id] = &channelsCacheEntry{
		Channels:  channels,
		NextFetch: nextFetch,
		SavedAt:   time.Now().Unix(),
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.channelsCachePath(), b, 0600)
}

func (s *Server) loadChannelsCache() *channelsCacheEntry {
	id := s.activeProfileID()
	if id == "" {
		return nil
	}
	return s.readChannelsCacheFile()[id]
}

func (s *Server) dropChannelsCacheEntry(profileID string) {
	if profileID == "" {
		return
	}
	f := s.readChannelsCacheFile()
	if _, ok := f[profileID]; !ok {
		return
	}
	delete(f, profileID)
	if len(f) == 0 {
		_ = os.Remove(s.channelsCachePath())
		return
	}
	if b, err := json.MarshalIndent(f, "", "  "); err == nil {
		_ = os.WriteFile(s.channelsCachePath(), b, 0600)
	}
}

// loadLastScan reads the most recent resolver scan result.
// Returns nil when the file doesn't exist or is older than 24 hours.
func (s *Server) loadLastScan() *lastScanData {
	b, err := os.ReadFile(filepath.Join(s.dataDir, "last_scan.json"))
	if err != nil {
		return nil
	}
	var d lastScanData
	if err := json.Unmarshal(b, &d); err != nil {
		return nil
	}
	if len(d.Resolvers) == 0 || time.Since(time.Unix(d.ScannedAt, 0)) > 24*time.Hour {
		return nil
	}
	return &d
}

func (s *Server) saveConfig(cfg *Config) error {
	path := filepath.Join(s.dataDir, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (s *Server) loadProfiles() (*ProfileList, error) {
	path := filepath.Join(s.dataDir, "profiles.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pl ProfileList
	if err := json.Unmarshal(data, &pl); err != nil {
		return nil, err
	}
	return &pl, nil
}

// loadProfilesExisting returns (nil, nil) only when the file truly
// doesn't exist; other errors are surfaced so callers don't overwrite
// with an empty struct.
func (s *Server) loadProfilesExisting() (*ProfileList, error) {
	path := filepath.Join(s.dataDir, "profiles.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var pl ProfileList
	if err := json.Unmarshal(data, &pl); err != nil {
		return nil, err
	}
	return &pl, nil
}

func (s *Server) saveProfiles(pl *ProfileList) error {
	if err := os.MkdirAll(s.dataDir, 0700); err != nil {
		return err
	}
	path := filepath.Join(s.dataDir, "profiles.json")
	data, err := json.MarshalIndent(pl, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
