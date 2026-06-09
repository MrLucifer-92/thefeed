package server

import (
	"testing"
	"time"

	"github.com/sartoopjj/thefeed/internal/protocol"
)

func newTestDNSServer(t *testing.T, domain string) *DNSServer {
	t.Helper()
	var qk, rk [protocol.KeySize]byte
	feed := NewFeed([]string{"c"})
	return NewDNSServer(":0", domain, feed, qk, rk, 0, nil, false, "", nil, false)
}

func TestDNSMatchDomain(t *testing.T) {
	s := newTestDNSServer(t, "t.example.com")
	// Blanks, the main domain, a trailing dot, and duplicates are ignored.
	s.SetExtraDomains([]string{"a.example.net", "t.example.com", " ", "b.example.org.", "a.example.net"})
	if len(s.domains) != 3 {
		t.Fatalf("domains = %v, want 3 (main + 2 unique extras)", s.domains)
	}

	cases := map[string]string{
		"abc.t.example.com":  "t.example.com",
		"xyz.a.example.net.": "a.example.net", // trailing dot tolerated
		"q.b.example.org":    "b.example.org",
		"T.EXAMPLE.COM":      "t.example.com", // case-insensitive
	}
	for qn, want := range cases {
		got, ok := s.matchDomain(qn)
		if !ok || got != want {
			t.Errorf("matchDomain(%q) = %q,%v want %q", qn, got, ok, want)
		}
	}
	if _, ok := s.matchDomain("foo.other.com"); ok {
		t.Error("expected no match for a foreign domain")
	}
}

// TestMultiDomainEncodeMatchDecode ties the client encode side to the server
// match+decode side: a query name built for ANY configured domain (main or an
// entirely separate extra domain) must be recognised by matchDomain and decode
// back to the original channel/block.
func TestMultiDomainEncodeMatchDecode(t *testing.T) {
	qk, rk, err := protocol.DeriveKeys("test-pp")
	if err != nil {
		t.Fatal(err)
	}
	feed := NewFeed([]string{"c"})
	s := NewDNSServer(":0", "t.example.com", feed, qk, rk, 0, nil, false, "", nil, false)
	s.SetExtraDomains([]string{"nws2.other.net"}) // a wholly separate base domain

	for _, dom := range []string{"t.example.com", "nws2.other.net"} {
		const ch, blk uint16 = 5, 2
		name, err := protocol.EncodeQuery(qk, ch, blk, dom, protocol.QuerySingleLabel)
		if err != nil {
			t.Fatalf("encode on %s: %v", dom, err)
		}
		matched, ok := s.matchDomain(name)
		if !ok || matched != dom {
			t.Fatalf("matchDomain(%q) = %q,%v want %q", name, matched, ok, dom)
		}
		gotCh, gotBlk, err := protocol.DecodeQuery(qk, name, matched)
		if err != nil {
			t.Fatalf("decode on %s: %v", dom, err)
		}
		if gotCh != ch || gotBlk != blk {
			t.Errorf("decode = ch%d blk%d, want ch%d blk%d", gotCh, gotBlk, ch, blk)
		}
	}
}

// TestRecordReportQueryPerDomain checks the per-domain totals: regular,
// metadata, version and media queries all count toward perDomain; an empty
// domain is skipped; an invalid query counts nowhere but invalidQueries.
func TestRecordReportQueryPerDomain(t *testing.T) {
	rep := newHourlyFetchReport(time.Now())
	recordReportQuery(rep, reportEvent{channel: 3, resolver: "1.1.1.1:53", domain: "a.example.com"})
	recordReportQuery(rep, reportEvent{channel: protocol.MetadataChannel, domain: "a.example.com"})
	recordReportQuery(rep, reportEvent{channel: protocol.VersionChannel, domain: "b.example.com"})
	recordReportQuery(rep, reportEvent{channel: protocol.MediaChannelStart, domain: "b.example.com"})
	recordReportQuery(rep, reportEvent{channel: 4, domain: ""}) // counted in total, not per-domain
	recordReportQuery(rep, reportEvent{invalid: true, domain: "a.example.com"})

	if rep.perDomain["a.example.com"] != 2 {
		t.Errorf("a.example.com = %d, want 2", rep.perDomain["a.example.com"])
	}
	if rep.perDomain["b.example.com"] != 2 {
		t.Errorf("b.example.com = %d, want 2", rep.perDomain["b.example.com"])
	}
	if _, ok := rep.perDomain[""]; ok {
		t.Error("empty domain must not be recorded")
	}
	if rep.totalQueries != 5 {
		t.Errorf("totalQueries = %d, want 5", rep.totalQueries)
	}
	if rep.invalidQueries != 1 {
		t.Errorf("invalidQueries = %d, want 1", rep.invalidQueries)
	}
}

func TestDNSMatchDomainLongestSuffix(t *testing.T) {
	// A sub-domain nested under the main domain must match the more specific
	// one so the correct suffix is stripped before decoding.
	s := newTestDNSServer(t, "example.com")
	s.SetExtraDomains([]string{"sub.example.com"})
	if got, _ := s.matchDomain("x.sub.example.com"); got != "sub.example.com" {
		t.Errorf("longest-suffix match = %q, want sub.example.com", got)
	}
	if got, _ := s.matchDomain("x.example.com"); got != "example.com" {
		t.Errorf("match = %q, want example.com", got)
	}
}
