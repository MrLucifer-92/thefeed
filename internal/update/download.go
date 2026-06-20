package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var downloadClient = &http.Client{
	Timeout: 10 * time.Minute,
}

// StreamAsset downloads version+asset from GitHub Releases and writes
// the body to dst with attachment Content-Disposition.
func StreamAsset(ctx context.Context, version, asset string, dst http.ResponseWriter, logf func(string)) error {
	if version == "" || asset == "" {
		return errors.New("StreamAsset: version and asset required")
	}
	return streamAssetFromURL(ctx, BaseURL+"/"+version+"/"+asset, asset, dst, logf)
}

func streamAssetFromURL(ctx context.Context, githubURL, asset string, dst http.ResponseWriter, logf func(string)) error {
	if logf == nil {
		logf = func(string) {}
	}

	logf(fmt.Sprintf("update: download starting %s", githubURL))
	body, contentLen, err := getFollowing(ctx, downloadClient, githubURL)
	if err != nil {
		logf(fmt.Sprintf("update: download failed: %v", err))
		return err
	}
	defer body.Close()
	logf(fmt.Sprintf("update: asset GET ok, Content-Length=%d", contentLen))

	name := sanitizeFilenameASCII(asset)
	dst.Header().Set("Content-Type", "application/octet-stream")
	dst.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	dst.Header().Set("X-Download-Filename", name)
	dst.Header().Set("Cache-Control", "no-store")
	if contentLen > 0 {
		dst.Header().Set("Content-Length", strconv.FormatInt(contentLen, 10))
	}

	start := time.Now()
	n, err := copyCapped(dst, body, contentLen)
	if err != nil {
		logf(fmt.Sprintf("update: download failed after %d/%d bytes: %v",
			n, contentLen, err))
		return err
	}
	logf(fmt.Sprintf("update: download complete — %d bytes in %s",
		n, time.Since(start).Round(time.Millisecond)))
	return nil
}

// getFollowing issues a GET, follows redirects, and returns the open body
// + Content-Length. Caller must close the body.
func getFollowing(ctx context.Context, c *http.Client, url string) (io.ReadCloser, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "*/*")
	resp, err := c.Do(req)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode/100 != 2 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		return nil, 0, fmt.Errorf("%s: %s",
			resp.Status, strings.TrimSpace(string(errBody)))
	}
	return resp.Body, resp.ContentLength, nil
}

// copyCapped stops after `expected` bytes if expected > 0, otherwise
// falls back to io.Copy until EOF.
func copyCapped(dst io.Writer, src io.Reader, expected int64) (int64, error) {
	if expected <= 0 {
		return io.Copy(dst, src)
	}
	return io.CopyN(dst, src, expected)
}

// sanitizeFilenameASCII keeps printable ASCII minus chars that would
// break a Content-Disposition quoted-string.
func sanitizeFilenameASCII(name string) string {
	if name == "" {
		return "thefeed-update.bin"
	}
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c < 0x20 || c > 0x7e || c == '"' || c == '\\' {
			continue
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return "thefeed-update.bin"
	}
	return string(out)
}
