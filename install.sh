#!/usr/bin/env bash
#
# anfra installer.
#
#   curl -fsSL https://raw.githubusercontent.com/holistics/anfra/main/install.sh | bash
#
# Downloads the anfra release binary for this platform from GitHub Releases and
# installs it to ~/.anfra/bin (override with ANFRA_INSTALL_DIR). The release
# binary embeds both sidecars, so it's large (~250 MB).
#
# Environment:
#   ANFRA_INSTALL_DIR    install location (default: $HOME/.anfra/bin)
#   ANFRA_VERSION        pin a version, e.g. 0.1.0 (default: latest)
#   ANFRA_NO_MODIFY_PATH if set, don't touch shell rc files; just print the hint
#
# Downloads from public GitHub Releases (no auth). Trust model: downloads over
# HTTPS from GitHub Releases (TOFU). Signature
# verification is not done here yet — `anfra update` is where verification will
# live (see .agents/projects/anfra/signing.md).

set -euo pipefail

REPO="holistics/anfra"
BIN_NAME="anfra"
INSTALL_DIR="${ANFRA_INSTALL_DIR:-${HOME}/.anfra/bin}"

err() { echo "anfra-install: $*" >&2; exit 1; }

# --- detect platform, mapped to the release asset names (anfra-<os>-<arch>) ---
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
    linux)  os="linux"  ;;
    darwin) os="darwin" ;;
    *)      err "unsupported OS: $os (anfra supports linux and macOS)" ;;
esac

arch="$(uname -m)"
case "$arch" in
    x86_64|amd64)  arch="x64"   ;;
    arm64|aarch64) arch="arm64" ;;
    *)             err "unsupported architecture: $arch" ;;
esac

asset="${BIN_NAME}-${os}-${arch}"

# --- resolve the download URL (avoid the GitHub API + its 60 req/hr limit) ---
# The /releases/latest/download/<asset> and /releases/download/<tag>/<asset>
# endpoints 302 straight to the CDN, so no API call is needed.
if [ -n "${ANFRA_VERSION:-}" ]; then
    tag="${ANFRA_VERSION#anfra-v}"; tag="anfra-v${tag#v}"
    url="https://github.com/${REPO}/releases/download/${tag}/${asset}"
else
    url="https://github.com/${REPO}/releases/latest/download/${asset}"
fi

# --- download ---
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
echo "Downloading ${asset} (~250 MB) for ${os}/${arch}..."
if ! curl -fL -o "${tmp}/${BIN_NAME}" "$url"; then
    err "download failed from ${url} (is the release published for this platform?)"
fi

# Log the checksum for the record (TOFU; no verification yet).
if command -v sha256sum >/dev/null 2>&1; then
    echo "SHA256: $(sha256sum "${tmp}/${BIN_NAME}" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
    echo "SHA256: $(shasum -a 256 "${tmp}/${BIN_NAME}" | awk '{print $1}')"
fi

chmod +x "${tmp}/${BIN_NAME}"
# macOS: the binary is unsigned, so clear the Gatekeeper quarantine flag.
if [ "$os" = "darwin" ] && command -v xattr >/dev/null 2>&1; then
    xattr -d com.apple.quarantine "${tmp}/${BIN_NAME}" 2>/dev/null || true
fi

# --- install ---
mkdir -p "$INSTALL_DIR"
mv -f "${tmp}/${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}"
target="${INSTALL_DIR}/${BIN_NAME}"
[ -x "$target" ] || err "installation failed: $target is not executable"

version="$("$target" --version 2>/dev/null || echo "installed")"
echo "Installed ${version} to ${target}"

# --- ensure INSTALL_DIR is on PATH ---
path_hint() {
    echo
    echo "Add ${INSTALL_DIR} to your PATH:"
    echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
}

ensure_on_path() {
    # Already reachable — nothing to do.
    case ":${PATH}:" in *":${INSTALL_DIR}:"*) return 0 ;; esac

    if [ -n "${ANFRA_NO_MODIFY_PATH:-}" ]; then
        path_hint
        return 0
    fi

    current="$(basename "${SHELL:-}")"
    updated=""
    # "<shell>:<rc file>:<line to add>" — edit an rc file when it already exists,
    # or when it belongs to the user's current shell (created if missing). The
    # "bash-login" rows cover macOS, where login shells read .bash_profile/.profile
    # instead of .bashrc; they're only edited if present (never created, so we
    # don't shadow an existing .profile).
    for entry in \
        "bash:${HOME}/.bashrc:export PATH=\"${INSTALL_DIR}:\$PATH\"" \
        "bash-login:${HOME}/.bash_profile:export PATH=\"${INSTALL_DIR}:\$PATH\"" \
        "bash-login:${HOME}/.profile:export PATH=\"${INSTALL_DIR}:\$PATH\"" \
        "zsh:${HOME}/.zshrc:export PATH=\"${INSTALL_DIR}:\$PATH\"" \
        "fish:${HOME}/.config/fish/config.fish:fish_add_path \"${INSTALL_DIR}\""
    do
        shell="${entry%%:*}"; rest="${entry#*:}"; rc="${rest%%:*}"; line="${rest#*:}"
        if [ -f "$rc" ] || [ "$shell" = "$current" ]; then
            mkdir -p "$(dirname "$rc")"
            if ! { [ -f "$rc" ] && grep -qF "$line" "$rc"; }; then
                printf '\n# Added by anfra installer\n%s\n' "$line" >> "$rc"
                echo "Added ${INSTALL_DIR} to PATH in ${rc}"
            fi
            updated="yes"
        fi
    done

    if [ -n "$updated" ]; then
        echo "Restart your shell (or 'source' the file above) to use \`anfra\`."
    else
        path_hint
    fi
}

ensure_on_path
