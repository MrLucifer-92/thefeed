package web

import (
	"strings"
)

// migrateResolverBank merges per-profile resolvers into the shared bank on first run.
//
// TODO(v1): remove this one-time migration once all clients have upgraded
// past the pre-shared-bank format; it only matters for configs created before
// the resolver bank existed.
func (s *Server) migrateResolverBank() {
	pl, err := s.loadProfiles()
	if err != nil || pl == nil {
		return
	}
	if len(pl.ResolverBank) > 0 {
		return // already migrated
	}
	seen := make(map[string]bool)
	for _, p := range pl.Profiles {
		for _, r := range p.Config.Resolvers {
			addr := r
			if !strings.Contains(addr, ":") {
				addr += ":53"
			}
			if !seen[addr] {
				seen[addr] = true
				pl.ResolverBank = append(pl.ResolverBank, addr)
			}
		}
	}
	if len(pl.ResolverBank) > 0 {
		for i := range pl.Profiles {
			pl.Profiles[i].Config.Resolvers = nil
		}
		_ = s.saveProfiles(pl)
	}
}

// addToBank adds resolvers to the shared bank (deduplicated, normalized with :53).
func addToBank(pl *ProfileList, resolvers []string) int {
	seen := make(map[string]bool)
	for _, r := range pl.ResolverBank {
		seen[r] = true
	}
	added := 0
	for _, r := range resolvers {
		addr := r
		if !strings.Contains(addr, ":") {
			addr += ":53"
		}
		if !seen[addr] {
			seen[addr] = true
			pl.ResolverBank = append(pl.ResolverBank, addr)
			added++
		}
	}
	return added
}

// persistResolverScores saves the current fetcher stats to profiles.json.
// Not serialised by profilesMu — initFetcher holds s.mu while calling this,
// and grabbing profilesMu here would risk AB-BA with handlers that take
// profilesMu first. The score map-merge is benign under last-writer-wins.
func (s *Server) persistResolverScores(stats map[string][3]int64) {
	if len(stats) == 0 {
		return
	}
	pl, err := s.loadProfiles()
	if err != nil || pl == nil {
		return
	}
	if pl.ResolverScores == nil {
		pl.ResolverScores = make(map[string]*SavedResolverScore)
	}
	for addr, st := range stats {
		pl.ResolverScores[addr] = &SavedResolverScore{
			Success: st[0],
			Failure: st[1],
			TotalMs: st[2],
		}
	}
	_ = s.saveProfiles(pl)
}

// computeResolverScore mirrors the scoring formula from fetcher.go.
func computeResolverScore(success, failure, totalMs int64) float64 {
	total := success + failure
	if total == 0 {
		return 0.2
	}
	successRate := float64(success) / float64(total)
	var avgMs float64
	if success > 0 {
		avgMs = float64(totalMs) / float64(success)
	} else {
		avgMs = 30000
	}
	score := successRate * successRate / (avgMs/5000.0 + 1.0)
	if score < 0.001 {
		score = 0.001
	}
	return score
}
