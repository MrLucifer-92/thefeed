package protocol

import (
	"strings"
	"testing"
)

func newTestKey(t *testing.T, pass string) [KeySize]byte {
	t.Helper()
	qk, _, err := DeriveKeys(pass)
	if err != nil {
		t.Fatalf("DeriveKeys: %v", err)
	}
	return qk
}

// Same (key, channel, block, domain, mode, seed) always produces the same name.
func TestEncodeQueryDeterministic_Stable(t *testing.T) {
	qk := newTestKey(t, "shared")
	seed := []byte{1, 2, 3, 4}
	for _, mode := range []QueryEncoding{QuerySingleLabel, QueryMultiLabel} {
		a, err := EncodeQueryDeterministic(qk, 5, 0, "t.example.com", mode, seed)
		if err != nil {
			t.Fatalf("mode=%d: %v", mode, err)
		}
		// Call ten times — any rand.Read leakage would surface quickly.
		for i := 0; i < 10; i++ {
			b, err := EncodeQueryDeterministic(qk, 5, 0, "t.example.com", mode, seed)
			if err != nil {
				t.Fatalf("mode=%d iter=%d: %v", mode, i, err)
			}
			if a != b {
				t.Errorf("mode=%d not stable: %q vs %q", mode, a, b)
			}
		}
	}
}

// Different seed → different name (so resolver-cache flips on epoch change).
func TestEncodeQueryDeterministic_SeedChange(t *testing.T) {
	qk := newTestKey(t, "shared")
	a, _ := EncodeQueryDeterministic(qk, 5, 0, "t.example.com", QuerySingleLabel, []byte{1})
	b, _ := EncodeQueryDeterministic(qk, 5, 0, "t.example.com", QuerySingleLabel, []byte{2})
	if a == b {
		t.Errorf("expected different names for different seeds, both = %q", a)
	}
}

// Sweeping the seed across many values must not collide for the same
// (channel, block, domain). 200 distinct seeds, expecting 200 distinct names.
func TestEncodeQueryDeterministic_SeedSweep(t *testing.T) {
	qk := newTestKey(t, "shared")
	seen := map[string]uint16{}
	for s := uint16(0); s < 200; s++ {
		seed := []byte{byte(s >> 8), byte(s)}
		q, err := EncodeQueryDeterministic(qk, 3, 0, "t.example.com", QuerySingleLabel, seed)
		if err != nil {
			t.Fatalf("seed=%d: %v", s, err)
		}
		if prev, ok := seen[q]; ok {
			t.Errorf("seed=%d collides with seed=%d: %q", s, prev, q)
		}
		seen[q] = s
	}
}

// Different (channel, block) tuples with the same seed must produce
// distinct names. Iterates a small grid so any cross-axis collision shows up.
func TestEncodeQueryDeterministic_ChannelBlockMatter(t *testing.T) {
	qk := newTestKey(t, "shared")
	seed := []byte{1, 2, 3, 4}
	type cb struct{ ch, blk uint16 }
	cases := []cb{
		{1, 0}, {2, 0}, {3, 0}, {1, 1}, {1, 2}, {2, 1}, {2, 2},
		{100, 0}, {100, 1}, {0xFFFA, 0}, {0xFFFA, 1},
	}
	seen := map[string]cb{}
	for _, c := range cases {
		q, err := EncodeQueryDeterministic(qk, c.ch, c.blk, "t.example.com", QuerySingleLabel, seed)
		if err != nil {
			t.Fatalf("channel=%d block=%d: %v", c.ch, c.blk, err)
		}
		if prev, ok := seen[q]; ok {
			t.Errorf("collision at ch=%d blk=%d (with ch=%d blk=%d): %q", c.ch, c.blk, prev.ch, prev.blk, q)
		}
		seen[q] = c
	}
}

// Different domain → different name. Defends against passphrase reuse across
// configs that point at different servers.
func TestEncodeQueryDeterministic_DomainInSeed(t *testing.T) {
	qk := newTestKey(t, "shared")
	seed := []byte{9}
	a, _ := EncodeQueryDeterministic(qk, 1, 0, "t.alice.com", QuerySingleLabel, seed)
	b, _ := EncodeQueryDeterministic(qk, 1, 0, "t.bob.com", QuerySingleLabel, seed)
	if a == b {
		t.Errorf("same encrypted blob across domains: %q", a)
	}
	if !strings.HasSuffix(a, ".t.alice.com") || !strings.HasSuffix(b, ".t.bob.com") {
		t.Errorf("wrong suffixes: a=%q b=%q", a, b)
	}
	// Strip the trailing domain and check the label part differs — i.e. the
	// encrypted blob itself reflects the domain mix-in, not just the suffix.
	labelA := strings.TrimSuffix(a, ".t.alice.com")
	labelB := strings.TrimSuffix(b, ".t.bob.com")
	if labelA == labelB {
		t.Errorf("label parts identical across domains: %q", labelA)
	}
}

// Different passphrase → completely different name even with identical
// (channel, block, domain, seed). Confirms the AES key still drives the bulk
// of the entropy.
func TestEncodeQueryDeterministic_KeyMatters(t *testing.T) {
	seed := []byte{1}
	qkA := newTestKey(t, "alice-pass")
	qkB := newTestKey(t, "bob-pass")
	a, _ := EncodeQueryDeterministic(qkA, 1, 0, "t.example.com", QuerySingleLabel, seed)
	b, _ := EncodeQueryDeterministic(qkB, 1, 0, "t.example.com", QuerySingleLabel, seed)
	if a == b {
		t.Errorf("different passphrases produced same name: %q", a)
	}
}

// Server can still decode a deterministic query into its (channel, block).
// Tested in both encoding modes.
func TestEncodeQueryDeterministic_Decodable(t *testing.T) {
	qk := newTestKey(t, "shared")
	seed := []byte{42}
	for _, mode := range []QueryEncoding{QuerySingleLabel, QueryMultiLabel} {
		q, err := EncodeQueryDeterministic(qk, 7, 3, "t.example.com", mode, seed)
		if err != nil {
			t.Fatalf("mode=%d encode: %v", mode, err)
		}
		ch, blk, err := DecodeQuery(qk, q, "t.example.com")
		if err != nil {
			t.Fatalf("mode=%d decode: %v", mode, err)
		}
		if ch != 7 || blk != 3 {
			t.Errorf("mode=%d decoded channel=%d block=%d, want 7/3", mode, ch, blk)
		}
	}
}

// Wrong-key decode must fail integrity check — the deterministic-suffix
// codepath shouldn't accidentally weaken that.
func TestEncodeQueryDeterministic_WrongKeyRejected(t *testing.T) {
	good := newTestKey(t, "alice")
	bad := newTestKey(t, "mallory")
	q, err := EncodeQueryDeterministic(good, 1, 0, "t.example.com", QuerySingleLabel, []byte{1})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if _, _, err := DecodeQuery(bad, q, "t.example.com"); err == nil {
		t.Error("expected decode to fail with wrong key")
	}
}

// Empty seed is rejected — caller should use EncodeQuery for random mode.
func TestEncodeQueryDeterministic_EmptySeed(t *testing.T) {
	qk := newTestKey(t, "shared")
	if _, err := EncodeQueryDeterministic(qk, 1, 0, "t.example.com", QuerySingleLabel, nil); err == nil {
		t.Error("expected error for nil seed")
	}
	if _, err := EncodeQueryDeterministic(qk, 1, 0, "t.example.com", QuerySingleLabel, []byte{}); err == nil {
		t.Error("expected error for empty seed")
	}
}

// Empty domain is rejected — same contract as the random encoder.
func TestEncodeQueryDeterministic_EmptyDomain(t *testing.T) {
	qk := newTestKey(t, "shared")
	if _, err := EncodeQueryDeterministic(qk, 1, 0, "", QuerySingleLabel, []byte{1}); err == nil {
		t.Error("expected error for empty domain")
	}
	if _, err := EncodeQueryDeterministic(qk, 1, 0, ".", QuerySingleLabel, []byte{1}); err == nil {
		t.Error("expected error for dot-only domain")
	}
}

// Trailing dot on the domain is normalised the same as the random encoder.
func TestEncodeQueryDeterministic_TrailingDot(t *testing.T) {
	qk := newTestKey(t, "shared")
	a, _ := EncodeQueryDeterministic(qk, 1, 0, "t.example.com", QuerySingleLabel, []byte{1})
	b, _ := EncodeQueryDeterministic(qk, 1, 0, "t.example.com.", QuerySingleLabel, []byte{1})
	if a != b {
		t.Errorf("trailing dot changed output: %q vs %q", a, b)
	}
}

// Multi-label split point is stable for the same input and varies across
// seeds — same property the random encoder has, just deterministically.
func TestEncodeQueryDeterministic_MultiLabelSplit(t *testing.T) {
	qk := newTestKey(t, "shared")
	// Stability: same input → same labels.
	a, _ := EncodeQueryDeterministic(qk, 1, 0, "t.example.com", QueryMultiLabel, []byte{1})
	b, _ := EncodeQueryDeterministic(qk, 1, 0, "t.example.com", QueryMultiLabel, []byte{1})
	if a != b {
		t.Errorf("split unstable: %q vs %q", a, b)
	}
	// Multi-label queries should have at least two labels before the domain.
	labels := strings.Count(strings.TrimSuffix(a, ".t.example.com"), ".")
	if labels < 1 {
		t.Errorf("expected multi-label output, got %q", a)
	}
}

// The random (default) encoder must produce a fresh name each call —
// otherwise enabling the feature would be a no-op everywhere.
func TestEncodeQuery_RandomIsNotStable(t *testing.T) {
	qk := newTestKey(t, "shared")
	a, _ := EncodeQuery(qk, 5, 0, "t.example.com", QuerySingleLabel)
	b, _ := EncodeQuery(qk, 5, 0, "t.example.com", QuerySingleLabel)
	if a == b {
		t.Errorf("random encoder produced identical names twice: %q", a)
	}
}

// Random and deterministic encoders must produce non-overlapping outputs
// for the same inputs — otherwise a deterministic client could be
// fingerprinted as having "accidentally hit" the random path.
func TestEncodeQuery_RandomVsDeterministic(t *testing.T) {
	qk := newTestKey(t, "shared")
	det, _ := EncodeQueryDeterministic(qk, 5, 0, "t.example.com", QuerySingleLabel, []byte{1})
	for i := 0; i < 10; i++ {
		rnd, _ := EncodeQuery(qk, 5, 0, "t.example.com", QuerySingleLabel)
		if rnd == det {
			t.Errorf("random %q collided with deterministic %q on iter %d", rnd, det, i)
		}
	}
}

// Metadata + write channels are excluded; everything else is eligible.
func TestChannelEligibleForSharedCache(t *testing.T) {
	excluded := []uint16{MetadataChannel, SendChannel, AdminChannel,
		UpstreamInitChannel, UpstreamDataChannel}
	for _, c := range excluded {
		if ChannelEligibleForSharedCache(c) {
			t.Errorf("channel %d should be excluded", c)
		}
	}
	eligible := []uint16{1, 5, 100, VersionChannel, TitlesChannel, RelayInfoChannel, ProfilePicsChannel}
	for _, c := range eligible {
		if !ChannelEligibleForSharedCache(c) {
			t.Errorf("channel %d should be eligible", c)
		}
	}
}
