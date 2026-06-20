package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sartoopjj/thefeed/internal/version"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.13.5", "v0.13.4", true},
		{"v0.13.5", "0.13.4", true},
		{"0.13.5", "v0.13.4", true},
		{"v0.13.5", "v0.13.5", false},
		{"v0.13.4", "v0.13.5", false},
		{"v1.0.0", "v0.99.99", true},
		{"v0.13.5", "dev", false},
		{"", "v0.13.5", false},
		{"v0.13.5", "", false},
		{"v0.13.5-rc1", "v0.13.4", true},
		{"v0.13.5", "v0.13.5-rc1", false}, // numeric parts equal → not newer
	}
	for _, c := range cases {
		if got := IsNewer(c.latest, c.current); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestAssetURLFromTemplate(t *testing.T) {
	old := version.AssetTemplate
	defer func() { version.AssetTemplate = old }()

	// GitHub Releases layout: BaseURL + "/{V}/{asset}".
	version.AssetTemplate = "thefeed-client-{V}-linux-amd64"
	url := AssetURL("v0.13.5")
	want := BaseURL + "/v0.13.5/thefeed-client-v0.13.5-linux-amd64"
	if url != want {
		t.Errorf("got %q, want %q", url, want)
	}

	version.AssetTemplate = "thefeed-client-{V}-windows-amd64.exe"
	url = AssetURL("v0.14.0")
	want = BaseURL + "/v0.14.0/thefeed-client-v0.14.0-windows-amd64.exe"
	if url != want {
		t.Errorf("got %q, want %q", url, want)
	}

	// Unversioned template (Android client binary) — {V} not present
	// in the asset name, but still appears as the per-release directory.
	version.AssetTemplate = "thefeed-client-android-arm64"
	url = AssetURL("v0.13.5")
	want = BaseURL + "/v0.13.5/thefeed-client-android-arm64"
	if url != want {
		t.Errorf("got %q, want %q", url, want)
	}
}

func TestAssetURLFallback(t *testing.T) {
	old := version.AssetTemplate
	defer func() { version.AssetTemplate = old }()
	version.AssetTemplate = ""

	url := AssetURL("v0.13.5")
	if url == "" {
		t.Fatal("expected non-empty URL even without AssetTemplate")
	}
	if !strings.HasPrefix(url, BaseURL+"/v0.13.5/") {
		t.Errorf("URL %q missing %q prefix", url, BaseURL+"/v0.13.5/")
	}
	// Should at minimum mention the running OS.
	if !strings.Contains(url, runtime.GOOS) && runtime.GOOS != "android" {
		t.Errorf("URL %q should mention %q", url, runtime.GOOS)
	}
}

// fetchLatestTag parses the tag out of the redirect Location.
func TestFetchLatestTag_ParsesLocation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/sartoopjj/thefeed/releases/tag/v0.19.0")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	c := &http.Client{CheckRedirect: stopOnRedirect, Timeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := fetchLatestTag(ctx, c, srv.URL)
	if err != nil {
		t.Fatalf("fetchLatestTag: %v", err)
	}
	if got != "v0.19.0" {
		t.Fatalf("got %q, want %q", got, "v0.19.0")
	}
}

// Non-3xx responses are errors — guards against future GitHub API
// shape changes silently returning an empty tag.
func TestFetchLatestTag_RejectsNonRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &http.Client{CheckRedirect: stopOnRedirect, Timeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := fetchLatestTag(ctx, c, srv.URL); err == nil {
		t.Fatal("expected error on 200 response")
	}
}

func TestMacAppTemplate(t *testing.T) {
	// DMG is universal (lipo'd arm64+amd64), so the template never
	// varies by GOARCH unlike the Android one.
	if got, want := macAppTemplate(), "thefeed-macos-{V}.dmg"; got != want {
		t.Errorf("macAppTemplate() = %q, want %q", got, want)
	}
}

func TestIsMacAppPath(t *testing.T) {
	cases := []struct {
		p    string
		want bool
	}{
		// Real-world install paths.
		{"/Applications/Thefeed.app/Contents/MacOS/thefeed-client", true},
		{"/Users/alice/Downloads/Thefeed.app/Contents/MacOS/thefeed-client", true},
		// Standalone CLI binaries (Homebrew, manual download, $GOPATH).
		{"/usr/local/bin/thefeed-client", false},
		{"/tmp/thefeed-client-v0.20.0-darwin-arm64", false},
		// .app dir but executable is outside Contents/MacOS/ — not us.
		{"/Applications/Other.app/Contents/Resources/helper", false},
		// Empty / odd inputs shouldn't panic and shouldn't match.
		{"", false},
	}
	for _, c := range cases {
		if got := isMacAppPath(c.p); got != c.want {
			t.Errorf("isMacAppPath(%q) = %v, want %v", c.p, got, c.want)
		}
	}
}

// On non-darwin platforms isMacApp must always return false so the
// build-time template (or defaultTemplate fallback) wins. This guards
// against an accidental refactor that drops the GOOS check.
func TestIsMacApp_NonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("test asserts non-darwin behavior")
	}
	if isMacApp() {
		t.Errorf("isMacApp() = true on %s, want false", runtime.GOOS)
	}
}

// Location must contain /tag/{V}; otherwise we have no way to know
// the version and should report an error rather than guess.
func TestFetchLatestTag_RejectsMalformedLocation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/totally/different/path")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	c := &http.Client{CheckRedirect: stopOnRedirect, Timeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := fetchLatestTag(ctx, c, srv.URL); err == nil {
		t.Fatal("expected error on malformed Location")
	}
}
