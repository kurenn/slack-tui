#!/bin/sh
# slack-tui installer — downloads the latest release binary for this machine,
# verifies its checksum, and installs it.
#
#   curl -fsSL https://raw.githubusercontent.com/kurenn/slack-tui/main/install.sh | sh
#
# Options (env vars):
#   VERSION=v0.1.0   install a specific version (default: latest)
#   BIN_DIR=~/bin    install directory (default: /usr/local/bin, falls back
#                    to ~/.local/bin when /usr/local/bin isn't writable)
set -eu

REPO="kurenn/slack-tui"

main() {
	os=$(uname -s | tr '[:upper:]' '[:lower:]')
	case "$os" in
	darwin | linux) ;;
	*)
		err "unsupported OS: $os — grab a binary from https://github.com/$REPO/releases"
		;;
	esac

	arch=$(uname -m)
	case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*)
		err "unsupported architecture: $arch"
		;;
	esac

	version="${VERSION:-$(latest_version)}"
	[ -n "$version" ] || err "could not determine the latest version"
	bare=${version#v}

	archive="slack-tui_${bare}_${os}_${arch}.tar.gz"
	base="https://github.com/$REPO/releases/download/$version"

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT

	say "downloading slack-tui $version ($os/$arch)…"
	fetch "$base/$archive" "$tmp/$archive"
	fetch "$base/checksums.txt" "$tmp/checksums.txt"

	say "verifying checksum…"
	(cd "$tmp" && grep " $archive\$" checksums.txt | sha256check) ||
		err "checksum verification failed"

	tar -xzf "$tmp/$archive" -C "$tmp" slack-tui

	bin_dir="${BIN_DIR:-/usr/local/bin}"
	if [ ! -w "$bin_dir" ] && [ -z "${BIN_DIR:-}" ]; then
		bin_dir="$HOME/.local/bin"
	fi
	mkdir -p "$bin_dir" 2>/dev/null || true
	install -m 0755 "$tmp/slack-tui" "$bin_dir/slack-tui" 2>/dev/null ||
		err "cannot write to $bin_dir — rerun with BIN_DIR=~/.local/bin or via sudo"

	say "installed $("$bin_dir/slack-tui" --version) → $bin_dir/slack-tui"
	case ":$PATH:" in
	*":$bin_dir:"*) ;;
	*) say "note: $bin_dir is not on your PATH — add: export PATH=\"\$PATH:$bin_dir\"" ;;
	esac
}

latest_version() {
	# the releases/latest redirect carries the tag; no API token needed
	curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" |
		sed 's|.*/tag/||'
}

fetch() {
	curl -fsSL "$1" -o "$2" || err "download failed: $1"
}

sha256check() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum -c - >/dev/null 2>&1
	else
		shasum -a 256 -c - >/dev/null 2>&1
	fi
}

say() { printf 'slack-tui: %s\n' "$1"; }
err() {
	printf 'slack-tui: error: %s\n' "$1" >&2
	exit 1
}

main "$@"
