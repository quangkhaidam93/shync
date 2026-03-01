#!/bin/sh
set -euf

REPO="quangkhaidam93/shync"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)       echo "Unsupported OS: $(uname -s)" >&2; exit 1 ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *)             echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
    esac
}

latest_version() {
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p'
    elif command -v wget >/dev/null 2>&1; then
        wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p'
    else
        echo "Error: curl or wget is required" >&2
        exit 1
    fi
}

download() {
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -o "$2" "$1"
    else
        wget -qO "$2" "$1"
    fi
}

main() {
    OS="$(detect_os)"
    ARCH="$(detect_arch)"

    if [ -n "${VERSION:-}" ]; then
        TAG="$VERSION"
    else
        echo "Fetching latest version..."
        TAG="$(latest_version)"
        if [ -z "$TAG" ]; then
            echo "Error: could not determine latest version" >&2
            exit 1
        fi
    fi

    VER="${TAG#v}"
    ARCHIVE="shync_${VER}_${OS}_${ARCH}.tar.gz"
    URL="https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE}"

    TMPDIR="$(mktemp -d)"
    trap 'rm -rf "$TMPDIR"' EXIT

    echo "Downloading shync ${TAG} for ${OS}/${ARCH}..."
    download "$URL" "${TMPDIR}/${ARCHIVE}"

    echo "Extracting..."
    tar -xzf "${TMPDIR}/${ARCHIVE}" -C "$TMPDIR"

    echo "Installing to ${INSTALL_DIR}/shync..."
    mkdir -p "$INSTALL_DIR"
    if [ -w "$INSTALL_DIR" ]; then
        mv "${TMPDIR}/shync" "${INSTALL_DIR}/shync"
    else
        sudo mv "${TMPDIR}/shync" "${INSTALL_DIR}/shync"
    fi
    chmod +x "${INSTALL_DIR}/shync"

    echo "shync ${TAG} installed successfully."
    echo "Run 'shync version' to verify."
}

main
