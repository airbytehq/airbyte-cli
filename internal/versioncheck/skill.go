package versioncheck

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	skillDirEnv      = "AIRBYTE_AGENT_SKILLS_DIR"
	defaultSkillsDir = ".claude/skills"
	skillName        = "airbyte-agent"
	skillFileName    = "SKILL.md"
)

// CheckSkill prints a one-line stderr warning when the installed
// airbyte-agent skill is older than the version this binary was built
// against. Silent in every other case: when disabled, when the output
// stream is not a terminal, when either version is a non-release tag
// (e.g. "dev"), when the skill file is missing, or when the installed
// version is equal to or newer than expected.
//
// Unlike Run, CheckSkill does no network I/O — the expected version is
// stamped into the binary at build time and the installed version lives
// on local disk.
func CheckSkill(expectedSkillVersion string, enabled bool, isTTY bool, stderr io.Writer) {
	if !enabled || !isTTY {
		return
	}
	expected, ok := parseVersion(expectedSkillVersion)
	if !ok {
		return
	}
	installedRaw, ok := readInstalledSkillVersion()
	if !ok {
		return
	}
	installed, ok := parseVersion(installedRaw)
	if !ok {
		return
	}
	if compareVersions(installed, expected) >= 0 {
		return
	}
	fmt.Fprint(stderr, formatSkillNudge(installedRaw, expectedSkillVersion))
}

// skillFrontmatterVersionRE pulls `metadata.version` out of the YAML
// frontmatter without pulling in a YAML dependency. It expects the
// indented `version:` to appear under a `metadata:` block. Quotes
// around the value are optional.
var skillFrontmatterVersionRE = regexp.MustCompile(`(?m)^metadata:\s*\n(?:[ \t]+[^\n]*\n)*?[ \t]+version:[ \t]*["']?([^"'\n]+?)["']?[ \t]*$`)

func readInstalledSkillVersion() (string, bool) {
	path := installedSkillPath()
	if path == "" {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	fm, ok := frontmatterBlock(data)
	if !ok {
		return "", false
	}
	m := skillFrontmatterVersionRE.FindSubmatch(fm)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(string(m[1])), true
}

// frontmatterBlock returns the bytes between the leading `---` fence and
// the next `---` fence. Returns false when the file lacks frontmatter.
func frontmatterBlock(data []byte) ([]byte, bool) {
	s := string(data)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return nil, false
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(s, "---\r\n"), "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, false
	}
	return []byte(rest[:end+1]), true
}

func installedSkillPath() string {
	if dir := strings.TrimSpace(os.Getenv(skillDirEnv)); dir != "" {
		return filepath.Join(dir, skillName, skillFileName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, defaultSkillsDir, skillName, skillFileName)
}

func formatSkillNudge(installed, expected string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Your airbyte-agent skill is %s — this CLI expects %s.\n", installed, expected)
	b.WriteString("  Reinstall with npx: npx skills add airbytehq/airbyte-agent-cli\n")
	b.WriteString("  Or reinstall via:   curl -fsSL https://airbyte.ai/install.sh | sh\n")
	b.WriteString("  (silence: set \"version_check_enabled\": false in ~/.airbyte-agent/settings.json)\n")
	return b.String()
}
