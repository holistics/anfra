#!/usr/bin/env bash
#
# anfra installer.
#
#   curl -fsSL https://anfra.ai/install.sh | bash
#
# Downloads the anfra release binary for this platform from GitHub Releases and
# installs it to ~/.anfra/bin (override with ANFRA_HOME or ANFRA_INSTALL_DIR). The release
# binary embeds both sidecars, so it's large (~250 MB).
#
# Environment:
#   ANFRA_HOME           anfra's folder, where it keeps everything (default: $HOME/.anfra)
#   ANFRA_INSTALL_DIR    install location (default: $ANFRA_HOME/bin)
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
INSTALL_DIR="${ANFRA_INSTALL_DIR:-${ANFRA_HOME:-${HOME}/.anfra}/bin}"

# --- output helpers (color only on a terminal, and never with NO_COLOR) ---
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    bold=$'\033[1m'; dim=$'\033[2m'; green=$'\033[32m'; yellow=$'\033[33m'; red=$'\033[31m'; reset=$'\033[0m'
else
    bold=""; dim=""; green=""; yellow=""; red=""; reset=""
fi

err()  { echo "${red}error${reset}: $*" >&2; exit 1; }
warn() { echo "${yellow}warning${reset}: $*" >&2; }
ok()   { echo "${green}✓${reset} $*"; }
# Show paths under $HOME as ~/..., which is shorter and easier to read. With an
# empty HOME every path would match "$HOME"/*, so leave paths as they are.
tilde() {
    if [ -n "$HOME" ]; then
        case "$1" in "$HOME"/*) echo "~${1#"$HOME"}"; return ;; esac
    fi
    echo "$1"
}

# --- required tools (fail early with a clear message, not mid-run) ---
command -v curl >/dev/null 2>&1 || err "curl is required but not found"
if command -v gunzip >/dev/null 2>&1; then
    gunzip_cmd="gunzip -c"
elif command -v gzip >/dev/null 2>&1; then
    gunzip_cmd="gzip -dc"
else
    err "gzip/gunzip is required to decompress the download but was not found"
fi

# --- detect platform, mapped to the release asset names (anfra-<os>-<arch>) ---
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
    linux)  os="linux";  os_label="Linux" ;;
    darwin) os="darwin"; os_label="macOS" ;;
    *)      err "unsupported OS: $os (anfra supports linux and macOS)" ;;
esac

arch="$(uname -m)"
case "$arch" in
    x86_64|amd64)  arch="x64"   ;;
    arm64|aarch64) arch="arm64" ;;
    *)             err "unsupported architecture: $arch" ;;
esac

# Release binaries are published gzip-compressed for transport (see the anfra
# compression plan); we gunzip after download.
asset="${BIN_NAME}-${os}-${arch}.gz"

# --- resolve the download URL (avoid the GitHub API + its 60 req/hr limit) ---
# The /releases/latest/download/<asset> and /releases/download/<tag>/<asset>
# endpoints 302 straight to the CDN, so no API call is needed.
if [ -n "${ANFRA_VERSION:-}" ]; then
    tag="${ANFRA_VERSION#anfra-v}"; tag="anfra-v${tag#v}"
    url="https://github.com/${REPO}/releases/download/${tag}/${asset}"
    wanted="anfra ${tag#anfra-v}"
else
    url="https://github.com/${REPO}/releases/latest/download/${asset}"
    wanted="the latest anfra"
fi

# --- download + decompress ---
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
echo
echo "Installing ${bold}${wanted}${reset} for ${os_label} ${arch}"
# A single progress bar on a terminal; quiet (errors only) when piped or in CI.
if [ -t 2 ]; then curl_progress="--progress-bar"; else curl_progress="-sS"; fi
if ! curl -fL $curl_progress -o "${tmp}/${BIN_NAME}.gz" "$url"; then
    err "download failed from ${url} (is the release published for this platform?)"
fi
if ! $gunzip_cmd "${tmp}/${BIN_NAME}.gz" > "${tmp}/${BIN_NAME}"; then
    err "failed to decompress ${asset}"
fi

# Keep the checksum for the record (TOFU; no verification yet); printed dimmed below.
sha=""
if command -v sha256sum >/dev/null 2>&1; then
    sha="$(sha256sum "${tmp}/${BIN_NAME}" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
    sha="$(shasum -a 256 "${tmp}/${BIN_NAME}" | awk '{print $1}')"
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

# `anfra --version` prints "anfra version X"; keep just the version.
version="$("$target" --version 2>/dev/null || true)"
version="${version#anfra version }"
echo
ok "Installed ${bold}anfra ${version:-}${reset} to $(tilde "$target")"
[ -n "$sha" ] && echo "  ${dim}sha256 ${sha}${reset}"

# --- ensure INSTALL_DIR is on PATH ---
# Set by ensure_on_path and read by the next steps below. updated is "yes" when
# an rc file has the PATH line; current_rc is the current shell's rc file with
# it, which the user can source to use anfra without restarting.
updated=""
current_rc=""

# For a line the user pastes: "$HOME" expands inside double quotes, "~" doesn't.
manual_path_hint() {
    local dir="$INSTALL_DIR"
    if [ -n "$HOME" ]; then
        case "$INSTALL_DIR" in "$HOME"/*) dir="\$HOME${INSTALL_DIR#"$HOME"}" ;; esac
    fi
    echo
    echo "$1 Run this now, and add it to your shell's startup file:"
    echo "  ${bold}export PATH=\"${dir}:\$PATH\"${reset}"
}

ensure_on_path() {
    local found current edited shell rest rc line

    # Already reachable. Warn if another anfra earlier on PATH would win.
    case ":${PATH}:" in *":${INSTALL_DIR}:"*)
        found="$(command -v "$BIN_NAME" 2>/dev/null || true)"
        if [ -n "$found" ] && [ "$found" != "$target" ]; then
            warn "another anfra at $(tilde "$found") comes before $(tilde "$target") on your PATH"
        fi
        return 0 ;;
    esac

    if [ -n "${ANFRA_NO_MODIFY_PATH:-}" ]; then
        manual_path_hint "$(tilde "$INSTALL_DIR") is not on your PATH."
        return 0
    fi

    current="$(basename "${SHELL:-}")"
    edited=""
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
                edited="${edited:+${edited}, }$(tilde "$rc")"
            fi
            updated="yes"
            [ "$shell" = "$current" ] && current_rc="$rc"
        fi
    done

    if [ -n "$edited" ]; then
        ok "Added $(tilde "$INSTALL_DIR") to PATH in ${edited}"
    fi
    if [ -z "$updated" ]; then
        manual_path_hint "Couldn't find a startup file for your shell (${current:-unknown}) to add $(tilde "$INSTALL_DIR") to PATH."
    fi
}

ensure_on_path

# --- what to do next ---
echo
if [ -n "$current_rc" ]; then
    echo "To start using anfra, restart your shell or run: ${bold}source $(tilde "$current_rc")${reset}"
    echo
elif [ -n "$updated" ]; then
    echo "To start using anfra, restart your shell."
    echo
fi
echo "${bold}Next steps${reset}"
echo
echo "  1. Install the Anfra skills. Claude Code, Codex, and Cursor need them"
echo "     to set up projects, model data, and build apps with Anfra:"
echo
echo "       ${bold}anfra skills install${reset}"
echo
echo "  2. Create a project:"
echo
echo "       ${bold}anfra init my-project${reset}"
echo "       ${bold}cd my-project${reset}"
echo
echo "  3. Connect your warehouse. Fill in your credentials in this file:"
echo
echo "       ${bold}.anfra/data_sources.yml${reset}"
echo
echo "     Supported warehouses and their connection fields are listed at"
echo "     https://docs.anfra.ai/docs/self-hosted/data-sources"
echo
echo "  4. Open the project in your coding agent and ask for an app, for example:"
echo
echo "       ${bold}Use the build-data-app skill. Look at my orders data and build a revenue${reset}"
echo "       ${bold}overview: monthly trend and revenue by region. Clicking a region should${reset}"
echo "       ${bold}filter the trend.${reset}"
echo
echo "     Then run ${bold}anfra serve${reset} and open http://127.0.0.1:7878/ to see it."
echo
echo "Docs: https://docs.anfra.ai"
