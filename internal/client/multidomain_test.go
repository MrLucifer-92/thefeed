package client

import "testing"

func TestPickDomainSingle(t *testing.T) {
	f := newTestFetcher(t, []string{"1.1.1.1:53"})
	// One domain → always the main domain, regardless of channel/block/attempt.
	for _, attempt := range []int{0, 1, 5} {
		if d := f.pickDomain(7, 3, attempt); d != f.domain {
			t.Errorf("single-domain pickDomain = %q, want %q", d, f.domain)
		}
	}
}

func TestSetDomainsDedup(t *testing.T) {
	f := newTestFetcher(t, []string{"1.1.1.1:53"}) // main = t.example.com
	f.SetDomains([]string{"a.example.com", "t.example.com", " ", "b.example.com.", "a.example.com"})
	// main + 2 unique extras; main stays first.
	if len(f.domains) != 3 || f.domains[0] != "t.example.com" {
		t.Fatalf("domains = %v, want [t.example.com a.example.com b.example.com]", f.domains)
	}
}

func TestPickDomainSpreadAndRotate(t *testing.T) {
	f := newTestFetcher(t, []string{"1.1.1.1:53"})
	f.SetDomains([]string{"a.example.com", "b.example.com"})
	if len(f.domains) != 3 {
		t.Fatalf("domains = %v, want 3", f.domains)
	}

	// Deterministic: same (channel, block, attempt) → same domain.
	if f.pickDomain(5, 2, 0) != f.pickDomain(5, 2, 0) {
		t.Error("pickDomain is not deterministic for identical inputs")
	}
	// Retry rotation: attempt+1 picks a different domain (n=3, +1 mod 3).
	if f.pickDomain(5, 2, 0) == f.pickDomain(5, 2, 1) {
		t.Error("expected attempt to rotate the domain")
	}
	// Every pick is one of the configured domains, and more than one is used.
	set := map[string]bool{}
	for ch := 0; ch < 64; ch++ {
		d := f.pickDomain(uint16(ch), 0, 0)
		known := false
		for _, dd := range f.domains {
			if dd == d {
				known = true
				break
			}
		}
		if !known {
			t.Fatalf("picked unknown domain %q", d)
		}
		set[d] = true
	}
	if len(set) < 2 {
		t.Errorf("expected queries to spread across domains, only used %v", set)
	}
}
