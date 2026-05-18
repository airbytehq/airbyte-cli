// Package versioncheck prints a one-line nudge to stderr when a newer
// release of airbyte agents is available on GitHub.
//
// The check is cached at ~/.airbyte-cli/version-check.json with a 24h
// TTL so a typical run does no network I/O. The first run on a fresh
// install fetches synchronously with a 1s timeout so the nudge can appear
// on that run; subsequent stale-cache refreshes happen in a background
// goroutine. All network and disk errors are silent — version checking
// must never affect the exit status or output of the user's command.
package versioncheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	cacheDirName  = ".airbyte-cli"
	cacheFileName = "version-check.json"
	cacheFileMode = 0o600
	cacheDirMode  = 0o700

	cacheTTL       = 24 * time.Hour
	networkTimeout = 1 * time.Second
)

// githubReleasesURL is overridable in tests.
var githubReleasesURL = "https://api.github.com/repos/airbytehq/airbyte-cli/releases/latest"

type cacheFile struct {
	LatestVersion string    `json:"latest_version"`
	CheckedAt     time.Time `json:"checked_at"`
	ReleaseURL    string    `json:"release_url"`
}

// Run inspects the cache, prints a nudge to stderr when the current
// version is behind, and refreshes the cache when it is missing or
// stale. It returns a channel that closes when any background refresh
// completes; callers (main) should wait on it with a bounded timeout so
// the cache write lands before the process exits. Returns nil when no
// background work was started.
//
// Run is a no-op when any of the following hold:
//   - enabled is false (user opted out via settings or env)
//   - isTTY is false (avoid polluting non-interactive streams)
//   - currentVersion is not a clean release tag (vX.Y.Z); "dev",
//     git-describe suffixes, and prereleases all skip the check
func Run(currentVersion string, enabled bool, isTTY bool, stderr io.Writer) <-chan struct{} {
	if !enabled || !isTTY {
		return nil
	}
	cur, ok := parseVersion(currentVersion)
	if !ok {
		return nil
	}

	cache, _ := readCache()

	if cache == nil {
		// First run on this machine — fetch synchronously so the nudge
		// can appear on this invocation.
		ctx, cancel := context.WithTimeout(context.Background(), networkTimeout)
		defer cancel()
		fetched, err := fetchLatest(ctx)
		if err != nil {
			return nil
		}
		_ = writeCache(fetched)
		maybeNudge(currentVersion, cur, fetched, stderr)
		return nil
	}

	maybeNudge(currentVersion, cur, cache, stderr)

	if time.Since(cache.CheckedAt) > cacheTTL {
		done := make(chan struct{})
		go func() {
			defer close(done)
			ctx, cancel := context.WithTimeout(context.Background(), networkTimeout)
			defer cancel()
			fetched, err := fetchLatest(ctx)
			if err != nil {
				return
			}
			_ = writeCache(fetched)
		}()
		return done
	}

	return nil
}

func maybeNudge(currentRaw string, current semver, c *cacheFile, stderr io.Writer) {
	latest, ok := parseVersion(c.LatestVersion)
	if !ok {
		return
	}
	if compareVersions(current, latest) >= 0 {
		return
	}
	fmt.Fprint(stderr, formatNudge(currentRaw, c.LatestVersion, c.ReleaseURL))
}

func formatNudge(current, latest, releaseURL string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "A new version of the airbyte CLI is available: %s (you have %s)\n", latest, current)
	b.WriteString("  Upgrade with brew: brew upgrade airbyte-cli\n")
	b.WriteString("  Or reinstall:      curl -fsSL https://airbyte.ai/install.sh | sh\n")
	if releaseURL != "" {
		fmt.Fprintf(&b, "  Release notes:     %s\n", releaseURL)
	}
	b.WriteString("  (silence: set \"version_check_enabled\": false in ~/.airbyte-cli/settings.json)\n")
	return b.String()
}

// IsTerminal reports whether f looks like an interactive terminal.
// Uses stat ModeCharDevice — works on macOS/Linux/Windows without a
// dependency on golang.org/x/term.
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// --- HTTP fetch --------------------------------------------------------

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

func fetchLatest(ctx context.Context) (*cacheFile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "airbyte-cli")

	client := &http.Client{Timeout: networkTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github releases returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}

	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("github releases returned empty tag_name")
	}
	if _, ok := parseVersion(rel.TagName); !ok {
		return nil, fmt.Errorf("github releases returned non-release tag %q", rel.TagName)
	}

	return &cacheFile{
		LatestVersion: rel.TagName,
		CheckedAt:     time.Now().UTC(),
		ReleaseURL:    rel.HTMLURL,
	}, nil
}

// --- Cache I/O ---------------------------------------------------------

func cachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, cacheDirName, cacheFileName)
}

func readCache() (*cacheFile, error) {
	path := cachePath()
	if path == "" {
		return nil, fmt.Errorf("no home directory")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c cacheFile
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.LatestVersion == "" {
		return nil, fmt.Errorf("cache missing latest_version")
	}
	return &c, nil
}

func writeCache(c *cacheFile) error {
	path := cachePath()
	if path == "" {
		return fmt.Errorf("no home directory")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, cacheDirMode); err != nil {
		return err
	}

	content, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')

	tmp, err := os.CreateTemp(dir, ".version-check-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(cacheFileMode); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// --- Semver parsing ----------------------------------------------------

type semver struct {
	major, minor, patch int
}

// strictReleaseRE matches vX.Y.Z with no prerelease or build suffix.
// We intentionally reject prerelease tags (vX.Y.Z-rc1), git-describe
// output (vX.Y.Z-3-gabc), and "dev".
var strictReleaseRE = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)

func parseVersion(s string) (semver, bool) {
	m := strictReleaseRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return semver{}, false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return semver{major, minor, patch}, true
}

func compareVersions(a, b semver) int {
	switch {
	case a.major != b.major:
		return a.major - b.major
	case a.minor != b.minor:
		return a.minor - b.minor
	default:
		return a.patch - b.patch
	}
}
