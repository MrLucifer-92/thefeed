package web

import "testing"

// nil or empty ProfileList must yield the documented defaults.
func TestConnectionSettings_Defaults(t *testing.T) {
	cases := map[string]*ProfileList{
		"nil":   nil,
		"empty": {},
	}
	for name, pl := range cases {
		t.Run(name, func(t *testing.T) {
			qm, rl, sc, to := connectionSettings(pl)
			if qm != defaultQueryMode {
				t.Errorf("queryMode = %q, want %q", qm, defaultQueryMode)
			}
			if rl != defaultRateLimit {
				t.Errorf("rateLimit = %v, want %v", rl, defaultRateLimit)
			}
			if sc != defaultScatter {
				t.Errorf("scatter = %v, want %v", sc, defaultScatter)
			}
			if to != defaultTimeout {
				t.Errorf("timeout = %v, want %v", to, defaultTimeout)
			}
		})
	}
}

// Non-zero fields override; unset fields fall back individually.
func TestConnectionSettings_Overrides(t *testing.T) {
	pl := &ProfileList{QueryMode: "double", RateLimit: 5, Scatter: 3, Timeout: 20}
	qm, rl, sc, to := connectionSettings(pl)
	if qm != "double" || rl != 5 || sc != 3 || to != 20 {
		t.Errorf("got (%v,%v,%v,%v) want (double,5,3,20)", qm, rl, sc, to)
	}

	// Partial override: only Timeout set; rest stay at defaults.
	pl2 := &ProfileList{Timeout: 25}
	qm, rl, sc, to = connectionSettings(pl2)
	if qm != defaultQueryMode || rl != defaultRateLimit || sc != defaultScatter || to != 25 {
		t.Errorf("partial override: got (%v,%v,%v,%v) want (%s,%v,%v,25)",
			qm, rl, sc, to, defaultQueryMode, defaultRateLimit, defaultScatter)
	}
}
