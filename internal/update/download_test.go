package update

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// stuckReader serves the buffer, then blocks on the next Read.
type stuckReader struct {
	data []byte
	pos  int
	hung *atomic.Bool
}

func (r *stuckReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		if r.hung != nil {
			r.hung.Store(true)
		}
		<-time.After(10 * time.Second)
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func TestCopyCapped_StopsAtContentLength(t *testing.T) {
	payload := bytes.Repeat([]byte{0xAB}, 1024)
	src := &stuckReader{data: payload}

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, err := copyCapped(&buf, src, int64(len(payload)))
		if err != nil {
			t.Errorf("copyCapped: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("copyCapped did not return after expected bytes")
	}

	if buf.Len() != len(payload) {
		t.Errorf("copied %d bytes, want %d", buf.Len(), len(payload))
	}
}

func TestCopyCapped_NoContentLength(t *testing.T) {
	src := strings.NewReader("hello world")
	var buf bytes.Buffer
	n, err := copyCapped(&buf, src, 0)
	if err != nil {
		t.Fatalf("copyCapped: %v", err)
	}
	if n != 11 || buf.String() != "hello world" {
		t.Errorf("got %q (%d bytes), want %q (11)", buf.String(), n, "hello world")
	}
}

func TestSanitizeFilenameASCII(t *testing.T) {
	cases := []struct{ in, want string }{
		{"thefeed-client-v0.19.1-darwin-arm64", "thefeed-client-v0.19.1-darwin-arm64"},
		{"thefeed-android-v0.19.1-arm64-v8a.apk", "thefeed-android-v0.19.1-arm64-v8a.apk"},
		{`evil"quote`, "evilquote"},
		{"path\\traversal", "pathtraversal"},
		{"new\nline", "newline"},
		{"", "thefeed-update.bin"},
		{"\x00\x01\x02", "thefeed-update.bin"},
	}
	for _, c := range cases {
		if got := sanitizeFilenameASCII(c.in); got != c.want {
			t.Errorf("sanitizeFilenameASCII(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStreamAsset_DirectSuccess(t *testing.T) {
	body := bytes.Repeat([]byte("A"), 4096)

	asset := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(200)
		w.Write(body)
	}))
	defer asset.Close()

	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", asset.URL)
		w.WriteHeader(302)
	}))
	defer gh.Close()

	rec := httptest.NewRecorder()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var logs []string
	err := streamAssetFromURL(ctx, gh.URL, "thefeed-client-v0.19.1-linux-amd64",
		rec, func(s string) { logs = append(logs, s) })
	if err != nil {
		t.Fatalf("StreamAsset: %v\nlogs: %v", err, logs)
	}
	if rec.Body.Len() != 4096 {
		t.Errorf("got %d bytes, want 4096", rec.Body.Len())
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "thefeed-client-v0.19.1-linux-amd64") {
		t.Errorf("Content-Disposition = %q, missing filename", got)
	}
	if got := rec.Header().Get("X-Download-Filename"); got != "thefeed-client-v0.19.1-linux-amd64" {
		t.Errorf("X-Download-Filename = %q", got)
	}
}
