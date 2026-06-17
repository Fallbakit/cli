#!/usr/bin/env sh
# Fallbakit CLI installer.
#
#   curl -fsSL https://fallbakit.com/install.sh | sh
#
# Downloads the latest release of the `fallbakit` CLI into ~/.fallbakit/bin.
# Override with FALLBAKIT_INSTALL_DIR and FALLBAKIT_VERSION (defaults to the
# latest GitHub release).
set -eu

REPO="fallbakit/cli"
INSTALL_DIR="${FALLBAKIT_INSTALL_DIR:-$HOME/.fallbakit/bin}"
VERSION="${FALLBAKIT_VERSION:-latest}"

info() { printf '\033[1m%s\033[0m\n' "$*"; }
err()  { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) err "unsupported architecture: $arch" ;;
esac
case "$os" in
  darwin|linux) ;;
  *) err "unsupported OS: $os (use the prebuilt Windows archive instead)" ;;
esac

if [ "$VERSION" = "latest" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name"' | head -n1 | cut -d '"' -f4)"
  [ -n "$VERSION" ] || err "could not determine the latest version"
fi
clean_version="${VERSION#v}"

archive="fallbakit_${clean_version}_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$VERSION/$archive"

info "Installing fallbakit $VERSION for $os/$arch"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$url" -o "$tmp/$archive" || err "download failed: $url"
tar -xzf "$tmp/$archive" -C "$tmp"

mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/fallbakit" "$INSTALL_DIR/fallbakit"

info "Installed to $INSTALL_DIR"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) printf '\nAdd it to your PATH:\n  export PATH="%s:$PATH"\n' "$INSTALL_DIR" ;;
esac
info "Run 'fallbakit login' to get started."
