#!/bin/sh
# Installs or updates clumyap on macOS and Linux:
#   curl -fsSL https://raw.githubusercontent.com/adamwahyuh/clumyap/main/install.sh | sh
# Files go to ~/.local/share/clumyap and the command to ~/.local/bin; no sudo needed.
# Uninstall: rm -rf ~/.local/share/clumyap ~/.local/bin/clumyap
set -eu

repo="adamwahyuh/clumyap"
share="${XDG_DATA_HOME:-$HOME/.local/share}/clumyap"
bin="$HOME/.local/bin"

case "$(uname -s)" in
Darwin) os=macos ;;
Linux) os=linux ;;
*) echo "clumyap: this installer is for macOS and Linux. On Windows run in PowerShell:" >&2
	echo "  irm https://raw.githubusercontent.com/$repo/main/install.ps1 | iex" >&2
	exit 1 ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) echo "clumyap: no build for $(uname -m) yet" >&2; exit 1 ;;
esac
# A shell running under Rosetta reports x86_64 on Apple Silicon; the native build is faster.
if [ "$os" = macos ] && [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null)" = 1 ]; then
	arch=arm64
fi

url="${CLUMYAP_URL:-https://github.com/$repo/releases/latest/download/clumyap-$os-$arch.tar.gz}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading clumyap for $os ($arch)..."
if ! curl -fSL --progress-bar "$url" -o "$tmp/clumyap.tar.gz"; then
	echo "clumyap: download failed: $url" >&2
	exit 1
fi
tar -xzf "$tmp/clumyap.tar.gz" -C "$tmp"
[ -x "$tmp/clumyap/clumyap" ] || { echo "clumyap: the download is not a clumyap archive" >&2; exit 1; }

mkdir -p "$(dirname "$share")" "$bin"
rm -rf "$share"
mv "$tmp/clumyap" "$share"
ln -sf "$share/clumyap" "$bin/clumyap"
# Files fetched by a browser are blocked on macOS until this is cleared; harmless otherwise.
[ "$os" = macos ] && xattr -dr com.apple.quarantine "$share" 2>/dev/null || true

version="$("$bin/clumyap" --version)"
echo "Installed $version to $share"

# Make sure the command is found in new terminals.
case ":$PATH:" in
*":$bin:"*) ;;
*)
	case "${SHELL:-}" in
	*/zsh) rc="$HOME/.zshrc" ;;
	*/bash) if [ "$os" = macos ]; then rc="$HOME/.bash_profile"; else rc="$HOME/.bashrc"; fi ;;
	*) rc="$HOME/.profile" ;;
	esac
	line='export PATH="$HOME/.local/bin:$PATH"'
	if ! grep -qsF "$line" "$rc"; then
		printf '\n# Added by the clumyap installer\n%s\n' "$line" >> "$rc"
		echo "Added ~/.local/bin to your PATH in $rc"
	fi
	echo "Open a new terminal window so the clumyap command is found."
	;;
esac

echo
echo "Get started:  clumyap start"
echo "The first run downloads the voice model (about 350 MB)."
