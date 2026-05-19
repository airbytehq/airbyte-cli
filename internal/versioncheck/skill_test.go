package versioncheck

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadInstalledSkillVersion_ParsesMetadataVersion(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, `---
name: airbyte-agent
description: foo
metadata:
  version: "v0.5.0"
---

# body
`)
	t.Setenv(skillDirEnv, dir)

	got, ok := readInstalledSkillVersion()
	if !ok {
		t.Fatalf("expected ok, got !ok")
	}
	if got != "v0.5.0" {
		t.Errorf("got %q, want v0.5.0", got)
	}
}

func TestReadInstalledSkillVersion_UnquotedValue(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, `---
name: airbyte-agent
metadata:
  version: v0.5.0
---
`)
	t.Setenv(skillDirEnv, dir)

	got, ok := readInstalledSkillVersion()
	if !ok || got != "v0.5.0" {
		t.Errorf("unquoted parse: got %q ok=%v, want v0.5.0", got, ok)
	}
}

func TestReadInstalledSkillVersion_MetadataPresentButNoVersion(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, `---
name: airbyte-agent
metadata:
  author: airbyte
---
`)
	t.Setenv(skillDirEnv, dir)

	if _, ok := readInstalledSkillVersion(); ok {
		t.Error("expected !ok when metadata has no version")
	}
}

func TestReadInstalledSkillVersion_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "# just a body, no frontmatter\nmetadata:\n  version: \"v0.5.0\"\n")
	t.Setenv(skillDirEnv, dir)

	if _, ok := readInstalledSkillVersion(); ok {
		t.Error("expected !ok when file lacks frontmatter")
	}
}

func TestReadInstalledSkillVersion_FileMissing(t *testing.T) {
	t.Setenv(skillDirEnv, t.TempDir())
	if _, ok := readInstalledSkillVersion(); ok {
		t.Error("expected !ok when SKILL.md absent")
	}
}

func TestReadInstalledSkillVersion_FallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(skillDirEnv, "") // explicitly unset
	writeSkillAt(t, filepath.Join(home, defaultSkillsDir, skillName, skillFileName), `---
name: airbyte-agent
metadata:
  version: "v0.9.9"
---
`)

	got, ok := readInstalledSkillVersion()
	if !ok || got != "v0.9.9" {
		t.Errorf("home fallback: got %q ok=%v, want v0.9.9", got, ok)
	}
}

func TestCheckSkill_DisabledSilent(t *testing.T) {
	seedSkill(t, "v0.4.0")
	var buf bytes.Buffer
	CheckSkill("v0.5.0", false, true, &buf)
	if buf.Len() != 0 {
		t.Errorf("disabled should be silent, got %q", buf.String())
	}
}

func TestCheckSkill_NonTTYSilent(t *testing.T) {
	seedSkill(t, "v0.4.0")
	var buf bytes.Buffer
	CheckSkill("v0.5.0", true, false, &buf)
	if buf.Len() != 0 {
		t.Errorf("non-TTY should be silent, got %q", buf.String())
	}
}

func TestCheckSkill_DevExpectedSilent(t *testing.T) {
	seedSkill(t, "v0.4.0")
	var buf bytes.Buffer
	CheckSkill("dev", true, true, &buf)
	if buf.Len() != 0 {
		t.Errorf("dev expected should be silent, got %q", buf.String())
	}
}

func TestCheckSkill_MissingSkillSilent(t *testing.T) {
	t.Setenv(skillDirEnv, t.TempDir())
	var buf bytes.Buffer
	CheckSkill("v0.5.0", true, true, &buf)
	if buf.Len() != 0 {
		t.Errorf("missing skill should be silent, got %q", buf.String())
	}
}

func TestCheckSkill_OlderInstalledNudges(t *testing.T) {
	seedSkill(t, "v0.4.0")
	var buf bytes.Buffer
	CheckSkill("v0.5.0", true, true, &buf)
	out := buf.String()
	if !strings.Contains(out, "v0.4.0") || !strings.Contains(out, "v0.5.0") {
		t.Errorf("nudge missing versions: %q", out)
	}
	if !strings.Contains(out, "npx skills add airbytehq/airbyte-agent-cli") {
		t.Errorf("nudge missing npx skills command: %q", out)
	}
	if !strings.Contains(out, "https://airbyte.ai/install.sh") {
		t.Errorf("nudge missing install URL: %q", out)
	}
}

func TestCheckSkill_EqualInstalledSilent(t *testing.T) {
	seedSkill(t, "v0.5.0")
	var buf bytes.Buffer
	CheckSkill("v0.5.0", true, true, &buf)
	if buf.Len() != 0 {
		t.Errorf("equal should be silent, got %q", buf.String())
	}
}

func TestCheckSkill_NewerInstalledSilent(t *testing.T) {
	seedSkill(t, "v0.6.0")
	var buf bytes.Buffer
	CheckSkill("v0.5.0", true, true, &buf)
	if buf.Len() != 0 {
		t.Errorf("newer should be silent, got %q", buf.String())
	}
}

func TestCheckSkill_DevInstalledSilent(t *testing.T) {
	// A skill bundle without a parseable version (e.g., dev build) must not crash or nudge.
	seedSkill(t, "dev")
	var buf bytes.Buffer
	CheckSkill("v0.5.0", true, true, &buf)
	if buf.Len() != 0 {
		t.Errorf("non-release installed version should be silent, got %q", buf.String())
	}
}

// --- helpers -----------------------------------------------------------

func seedSkill(t *testing.T, version string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(skillDirEnv, dir)
	writeSkill(t, dir, "---\nname: airbyte-agent\nmetadata:\n  version: \""+version+"\"\n---\n")
}

func writeSkill(t *testing.T, dir, body string) {
	t.Helper()
	writeSkillAt(t, filepath.Join(dir, skillName, skillFileName), body)
}

func writeSkillAt(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
