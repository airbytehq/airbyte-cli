package versioncheck

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in   string
		want semver
		ok   bool
	}{
		{"v1.2.3", semver{1, 2, 3}, true},
		{"v0.0.1", semver{0, 0, 1}, true},
		{"v10.20.30", semver{10, 20, 30}, true},
		{" v1.2.3 ", semver{1, 2, 3}, true},
		{"1.2.3", semver{}, false},       // missing leading v
		{"v1.2", semver{}, false},        // missing patch
		{"v1.2.3-rc1", semver{}, false},  // prerelease rejected
		{"v1.2.3+meta", semver{}, false}, // build metadata rejected
		{"vX.Y.Z", semver{}, false},
		{"dev", semver{}, false},
		{"", semver{}, false},
		// git-describe-style output is rejected (only clean release tags
		// trigger the check)
		{"v0.4.2-3-gabc1234", semver{}, false},
		{"v0.4.2-3-gabc1234-dirty", semver{}, false},
	}
	for _, tt := range tests {
		got, ok := parseVersion(tt.in)
		if ok != tt.ok {
			t.Errorf("parseVersion(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			continue
		}
		if ok && got != tt.want {
			t.Errorf("parseVersion(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b semver
		want int // -, 0, +
	}{
		{semver{1, 2, 3}, semver{1, 2, 3}, 0},
		{semver{1, 2, 3}, semver{1, 2, 4}, -1},
		{semver{1, 2, 4}, semver{1, 2, 3}, +1},
		{semver{1, 2, 3}, semver{1, 3, 0}, -1},
		{semver{2, 0, 0}, semver{1, 99, 99}, +1},
	}
	for _, c := range cases {
		got := compareVersions(c.a, c.b)
		sign := func(n int) int {
			switch {
			case n < 0:
				return -1
			case n > 0:
				return 1
			}
			return 0
		}
		if sign(got) != sign(c.want) {
			t.Errorf("compare(%v,%v)=%d, want sign %d", c.a, c.b, got, c.want)
		}
	}
}

func TestRun_DisabledDoesNothing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	var buf bytes.Buffer
	done := Run("v0.1.0", false, true, &buf)
	if done != nil {
		t.Errorf("expected nil channel when disabled")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output when disabled, got %q", buf.String())
	}
}

func TestRun_NonTTYDoesNothing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	var buf bytes.Buffer
	done := Run("v0.1.0", true, false, &buf)
	if done != nil {
		t.Errorf("expected nil channel when not a TTY")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output when not a TTY, got %q", buf.String())
	}
}

func TestRun_DevVersionSkipped(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Pre-seed cache so we'd otherwise nudge.
	seedCache(t, &cacheFile{LatestVersion: "v9.9.9", CheckedAt: time.Now(), ReleaseURL: "https://example.test"})

	var buf bytes.Buffer
	done := Run("dev", true, true, &buf)
	if done != nil {
		t.Errorf("expected nil channel for dev version")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output for dev version, got %q", buf.String())
	}
}

func TestRun_FreshCacheNoNudgeWhenCurrent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	seedCache(t, &cacheFile{LatestVersion: "v0.4.2", CheckedAt: time.Now(), ReleaseURL: "https://example.test"})

	var buf bytes.Buffer
	done := Run("v0.4.2", true, true, &buf)
	if done != nil {
		t.Errorf("expected nil channel — cache is fresh, no refresh needed")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no nudge when on latest, got %q", buf.String())
	}
}

func TestRun_FreshCacheNudgesWhenBehind(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	seedCache(t, &cacheFile{LatestVersion: "v0.5.0", CheckedAt: time.Now(), ReleaseURL: "https://example.test/r"})

	var buf bytes.Buffer
	done := Run("v0.4.2", true, true, &buf)
	if done != nil {
		t.Errorf("expected nil channel — cache is fresh, no refresh needed")
	}
	out := buf.String()
	if !strings.Contains(out, "v0.5.0") || !strings.Contains(out, "v0.4.2") {
		t.Errorf("nudge missing version info: %q", out)
	}
	if !strings.Contains(out, "https://airbyte.ai/install.sh") {
		t.Errorf("nudge missing install URL: %q", out)
	}
	if !strings.Contains(out, "https://example.test/r") {
		t.Errorf("nudge missing release URL: %q", out)
	}
}

func TestRun_StaleCacheTriggersBackgroundRefresh(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	seedCache(t, &cacheFile{LatestVersion: "v0.4.2", CheckedAt: time.Now().Add(-48 * time.Hour), ReleaseURL: "https://example.test/r"})

	srv := newReleasesServer(t, "v0.5.0", "https://example.test/r2")
	defer srv.Close()
	old := githubReleasesURL
	githubReleasesURL = srv.URL
	defer func() { githubReleasesURL = old }()

	var buf bytes.Buffer
	done := Run("v0.4.2", true, true, &buf)
	if done == nil {
		t.Fatalf("expected a refresh channel for stale cache")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("refresh did not complete within 2s")
	}

	// Cache file should now have the new tag.
	c, err := readCache()
	if err != nil {
		t.Fatalf("readCache after refresh: %v", err)
	}
	if c.LatestVersion != "v0.5.0" {
		t.Errorf("cache LatestVersion = %q, want v0.5.0", c.LatestVersion)
	}
	// The nudge printed before the refresh — based on the stale cache —
	// should still mention v0.4.2 as the install we need to upgrade FROM.
	// (The stale cache had v0.4.2 as latest, so no nudge would fire.)
	if buf.Len() != 0 {
		t.Errorf("expected no nudge when stale cache equals current, got %q", buf.String())
	}
}

func TestRun_NoCacheDoesSyncFetchAndNudges(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	srv := newReleasesServer(t, "v0.5.0", "https://example.test/r")
	defer srv.Close()
	old := githubReleasesURL
	githubReleasesURL = srv.URL
	defer func() { githubReleasesURL = old }()

	var buf bytes.Buffer
	done := Run("v0.4.2", true, true, &buf)
	if done != nil {
		t.Errorf("expected nil channel — first run uses sync fetch, no background goroutine")
	}

	out := buf.String()
	if !strings.Contains(out, "v0.5.0") {
		t.Errorf("expected nudge on first run after sync fetch, got %q", out)
	}

	c, err := readCache()
	if err != nil {
		t.Fatalf("readCache after first run: %v", err)
	}
	if c.LatestVersion != "v0.5.0" {
		t.Errorf("cache LatestVersion = %q, want v0.5.0", c.LatestVersion)
	}
}

func TestRun_NoCacheFetchErrorSilentlyContinues(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	old := githubReleasesURL
	githubReleasesURL = srv.URL
	defer func() { githubReleasesURL = old }()

	var buf bytes.Buffer
	done := Run("v0.4.2", true, true, &buf)
	if done != nil {
		t.Errorf("expected nil channel on fetch error")
	}
	if buf.Len() != 0 {
		t.Errorf("expected silent failure, got %q", buf.String())
	}
	if _, err := os.Stat(filepath.Join(tmp, ".airbyte-cli", "version-check.json")); !os.IsNotExist(err) {
		t.Errorf("expected no cache file on fetch failure, got err=%v", err)
	}
}

func TestFetchLatest_RejectsPrereleaseTags(t *testing.T) {
	// /releases/latest should never return a prerelease (GitHub filters
	// them out) but defensively reject if it does.
	srv := newReleasesServer(t, "v0.5.0-rc1", "https://example.test/r")
	defer srv.Close()
	old := githubReleasesURL
	githubReleasesURL = srv.URL
	defer func() { githubReleasesURL = old }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := fetchLatest(ctx); err == nil {
		t.Error("expected fetchLatest to reject non-release tag, got nil")
	}
}

func TestWriteCache_AtomicAndPermissions(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	c := &cacheFile{LatestVersion: "v0.4.2", CheckedAt: time.Now(), ReleaseURL: "https://example.test"}
	if err := writeCache(c); err != nil {
		t.Fatalf("writeCache: %v", err)
	}
	info, err := os.Stat(filepath.Join(tmp, ".airbyte-cli", "version-check.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissions = %o, want 0600", perm)
	}
}

// --- helpers -----------------------------------------------------------

func seedCache(t *testing.T, c *cacheFile) {
	t.Helper()
	if err := writeCache(c); err != nil {
		t.Fatalf("seedCache: %v", err)
	}
}

func newReleasesServer(t *testing.T, tag, htmlURL string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(githubRelease{TagName: tag, HTMLURL: htmlURL})
	}))
}
