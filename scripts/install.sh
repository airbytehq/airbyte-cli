#!/usr/bin/env bash
# Install the airbyte-agent CLI and its agent skills.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/airbytehq/airbyte-agent-cli/main/scripts/install.sh | bash
#
# Environment overrides:
#   AIRBYTE_AGENT_VERSION       Install a specific tag (e.g. v0.2.0). Default: latest release.
#   AIRBYTE_AGENT_INSTALL_DIR   CLI target directory. Default: /usr/local/bin if writable, else $HOME/.local/bin.
#   AIRBYTE_AGENT_SKILLS_DIR    Skills target directory. Default: $HOME/.claude/skills.
#   AIRBYTE_AGENT_SKIP_SKILLS   Set to 1 to skip skill installation.
#
# Supports macOS, Linux, and WSL on amd64/arm64. Downloads the goreleaser
# tarball from GitHub and verifies the SHA-256 against checksums.txt.

set -eu

REPO="airbytehq/airbyte-agent-cli"
BINARY="airbyte-agent"

if [ -t 1 ]; then
  BOLD=$(printf '\033[1m'); RED=$(printf '\033[31m')
  GREEN=$(printf '\033[32m'); YELLOW=$(printf '\033[33m')
  RESET=$(printf '\033[0m')
else
  BOLD=""; RED=""; GREEN=""; YELLOW=""; RESET=""
fi

info() { printf "%s\n" "$*"; }
ok()   { printf "%s✓%s %s\n" "$GREEN" "$RESET" "$*"; }
warn() { printf "%s!%s %s\n" "$YELLOW" "$RESET" "$*" >&2; }
err()  { printf "%serror:%s %s\n" "$RED" "$RESET" "$*" >&2; }
die()  { err "$@"; exit 1; }

require() {
  command -v "$1" >/dev/null 2>&1 || die "missing required tool: $1"
}

require curl
require tar
require uname

uname_s=$(uname -s)
uname_m=$(uname -m)

case "$uname_s" in
  Darwin) os=darwin ;;
  Linux)  os=linux ;;
  *)      die "unsupported OS: $uname_s (supported: macOS, Linux, WSL)" ;;
esac

case "$uname_m" in
  x86_64|amd64)  arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *)             die "unsupported architecture: $uname_m (supported: amd64, arm64)" ;;
esac

version="${AIRBYTE_AGENT_VERSION:-}"
if [ -z "$version" ]; then
  info "Looking up latest release..."
  version=$(curl -fsSL -H "Accept: application/vnd.github+json" \
    "https://api.github.com/repos/$REPO/releases/latest" \
    | sed -n 's/.*"tag_name":[[:space:]]*"\(v[^"]*\)".*/\1/p' \
    | head -n1)
  [ -n "$version" ] || die "could not resolve latest release; set AIRBYTE_AGENT_VERSION to a tag (e.g. v0.2.0)"
fi
case "$version" in v*) ;; *) version="v$version" ;; esac
version_no_v="${version#v}"

install_dir="${AIRBYTE_AGENT_INSTALL_DIR:-}"
if [ -z "$install_dir" ]; then
  if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    install_dir=/usr/local/bin
  else
    install_dir="$HOME/.local/bin"
  fi
fi
mkdir -p "$install_dir" || die "cannot create install dir: $install_dir"
[ -w "$install_dir" ] || die "install dir is not writable: $install_dir (set AIRBYTE_AGENT_INSTALL_DIR or rerun with sudo)"

skills_dir="${AIRBYTE_AGENT_SKILLS_DIR:-$HOME/.claude/skills}"
skip_skills="${AIRBYTE_AGENT_SKIP_SKILLS:-0}"

archive="${BINARY}_${version_no_v}_${os}_${arch}.tar.gz"
base_url="https://github.com/$REPO/releases/download/$version"
url="$base_url/$archive"
checksums_url="$base_url/checksums.txt"
source_url="https://codeload.github.com/$REPO/tar.gz/refs/tags/$version"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# --- CLI: download + verify ---

info "Downloading $archive ($version)..."
curl -fsSL --proto '=https' --tlsv1.2 -o "$tmp/$archive" "$url" \
  || die "failed to download $url"

info "Verifying checksum..."
if curl -fsSL --proto '=https' --tlsv1.2 -o "$tmp/checksums.txt" "$checksums_url"; then
  expected=$(awk -v f="$archive" '$2==f {print $1}' "$tmp/checksums.txt")
  if [ -z "$expected" ]; then
    warn "no checksum entry for $archive; skipping verification"
  elif command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$tmp/$archive" | awk '{print $1}')
    [ "$actual" = "$expected" ] || die "checksum mismatch: expected $expected, got $actual"
    ok "checksum verified"
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
    [ "$actual" = "$expected" ] || die "checksum mismatch: expected $expected, got $actual"
    ok "checksum verified"
  else
    warn "no sha256sum/shasum on PATH; skipping verification"
  fi
else
  warn "could not fetch checksums.txt; skipping verification"
fi

tar -xzf "$tmp/$archive" -C "$tmp" || die "failed to extract archive"
[ -f "$tmp/$BINARY" ] || die "binary '$BINARY' not found in archive"

target="$install_dir/$BINARY"
cp "$tmp/$BINARY" "$target" || die "failed to copy binary to $target"
chmod 0755 "$target" || die "failed to chmod $target"

ok "Installed ${BOLD}$BINARY $version${RESET} to $target"

# --- Skills ---

installed_skills=0
if [ "$skip_skills" = "1" ] || [ "$skip_skills" = "true" ]; then
  info "Skipping skills (AIRBYTE_AGENT_SKIP_SKILLS set)."
else
  info ""
  info "Installing skills to $skills_dir..."
  mkdir -p "$skills_dir" || die "cannot create skills dir: $skills_dir"
  [ -w "$skills_dir" ] || die "skills dir is not writable: $skills_dir (set AIRBYTE_AGENT_SKILLS_DIR)"

  src_tarball="$tmp/source.tar.gz"
  if ! curl -fsSL --proto '=https' --tlsv1.2 -o "$src_tarball" "$source_url"; then
    warn "could not download source tarball ($source_url); skipping skills"
  else
    src_dir="$tmp/src"
    mkdir -p "$src_dir"
    if ! tar -xzf "$src_tarball" -C "$src_dir"; then
      warn "could not extract source tarball; skipping skills"
    else
      # Tarball extracts to a single top-level dir like airbyte-agent-cli-0.2.0
      skills_src=$(find "$src_dir" -mindepth 2 -maxdepth 2 -type d -name skills | head -n1)
      if [ -z "$skills_src" ] || [ ! -d "$skills_src" ]; then
        warn "no skills/ directory found in source tarball; skipping"
      else
        for skill_path in "$skills_src"/*/; do
          [ -d "$skill_path" ] || continue
          skill_name=$(basename "$skill_path")
          rm -rf "$skills_dir/$skill_name"
          cp -R "$skill_path" "$skills_dir/$skill_name" \
            || die "failed to copy skill $skill_name to $skills_dir"
          installed_skills=$((installed_skills + 1))
        done
        ok "Installed $installed_skills skill$([ "$installed_skills" -eq 1 ] || echo s) to $skills_dir"
      fi
    fi
  fi
fi

# --- PATH hint + next steps ---

info ""
case ":$PATH:" in
  *":$install_dir:"*)
    info "Run ${BOLD}$BINARY login${RESET} to authenticate."
    ;;
  *)
    warn "$install_dir is not on your PATH."
    info "Add it by appending the following to your shell rc (~/.zshrc, ~/.bashrc):"
    info ""
    info "    export PATH=\"$install_dir:\$PATH\""
    info ""
    info "Then run ${BOLD}$BINARY login${RESET} to authenticate."
    ;;
esac
