package client

import (
	"testing"
	"time"
)

// Default state: no cooldown, no failures, extended path is open.
func TestExtCooldownInitial(t *testing.T) {
	f := newTestFetcher(t, []string{"1.1.1.1:53"})
	if f.extInCooldown() {
		t.Error("fresh fetcher should not be in cooldown")
	}
	if got := f.extFailCount.Load(); got != 0 {
		t.Errorf("extFailCount = %d, want 0", got)
	}
}

func TestExtCooldownBlocks(t *testing.T) {
	f := newTestFetcher(t, []string{"1.1.1.1:53"})
	f.extCooldownUntil.Store(time.Now().Add(metadataExtCooldownDur).Unix())
	if !f.extInCooldown() {
		t.Error("expected cooldown to be active")
	}
}

func TestExtCooldownPastExpiry(t *testing.T) {
	f := newTestFetcher(t, []string{"1.1.1.1:53"})
	f.extCooldownUntil.Store(time.Now().Add(-time.Hour).Unix())
	if f.extInCooldown() {
		t.Error("expired cooldown must not block")
	}
}

// Tuning constants must stay within sensible ranges so accidental edits
// don't silently widen the cooldown window or shrink the fallback cap.
func TestExtTuningConstants(t *testing.T) {
	if metadataExtMaxRetries <= 0 || metadataExtMaxRetries > 100 {
		t.Errorf("metadataExtMaxRetries out of sensible range: %d", metadataExtMaxRetries)
	}
	if metadataExtCooldownDur < time.Minute || metadataExtCooldownDur > time.Hour {
		t.Errorf("metadataExtCooldownDur out of sensible range: %v", metadataExtCooldownDur)
	}
	if metadataLegacyCap < 16 {
		t.Errorf("metadataLegacyCap too small: %d (must allow real metadata after block-size shrinks)", metadataLegacyCap)
	}
}
