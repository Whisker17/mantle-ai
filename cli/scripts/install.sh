#!/usr/bin/env sh
set -eu

TARGET_VERSION="${1:-}"
REPO="${MANTLE_REPO:-mantle/mantle-ai}"
BIN_NAME="mantle"
REQUESTED_VERSION="${MANTLE_VERSION:-${TARGET_VERSION:-latest}}"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1" >&2; exit 1; }
}

detect_os() {
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    linux) echo "linux" ;;
    darwin) echo "darwin" ;;
    *) echo "unsupported OS: $os" >&2; exit 1 ;;
  esac
}

detect_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
  esac
}

resolve_tag() {
  if [ "$REQUESTED_VERSION" = "latest" ] || [ "$REQUESTED_VERSION" = "stable" ]; then
    curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | awk -F\" '/"tag_name":/ {print $4; exit}'
    return
  fi
  case "$REQUESTED_VERSION" in
    v*) echo "$REQUESTED_VERSION" ;;
    [0-9]*) echo "v$REQUESTED_VERSION" ;;
    *) echo "invalid version '$REQUESTED_VERSION'" >&2; exit 1 ;;
  esac
}

need_cmd curl
need_cmd tar
need_cmd mktemp
need_cmd awk
need_cmd grep

OS="$(detect_os)"
ARCH="$(detect_arch)"
TAG="$(resolve_tag)"
VERSION="${TAG#v}"
ARCHIVE="${BIN_NAME}_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/$REPO/releases/download/$TAG"
TMP_DIR="$(mktemp -d)"
ARCHIVE_PATH="$TMP_DIR/$ARCHIVE"
CHECKSUMS_PATH="$TMP_DIR/checksums.txt"
BIN_PATH="$TMP_DIR/$BIN_NAME"
INSTALL_DIR="${MANTLE_INSTALL_DIR:-$HOME/.local/bin}"

cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT INT TERM

echo "Installing $BIN_NAME $TAG for $OS/$ARCH from $REPO..."
curl -fsSL "$BASE_URL/$ARCHIVE" -o "$ARCHIVE_PATH"
curl -fsSL "$BASE_URL/checksums.txt" -o "$CHECKSUMS_PATH"
grep "  $ARCHIVE$" "$CHECKSUMS_PATH" > "$TMP_DIR/expected.checksum"
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$TMP_DIR" && sha256sum -c expected.checksum >/dev/null)
else
  (cd "$TMP_DIR" && shasum -a 256 -c expected.checksum >/dev/null)
fi

tar -xzf "$ARCHIVE_PATH" -C "$TMP_DIR"
mkdir -p "$INSTALL_DIR"
cp "$BIN_PATH" "$INSTALL_DIR/$BIN_NAME"
chmod +x "$INSTALL_DIR/$BIN_NAME"

echo "Installed to $INSTALL_DIR/$BIN_NAME"
"$INSTALL_DIR/$BIN_NAME" version --long || true
