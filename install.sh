#!/bin/sh
# Installs or updates clumyap on macOS and Linux:
#   curl -fsSL https://raw.githubusercontent.com/adamwahyuh/clumyap/main/install.sh | sh
# Files go to ~/.local/share/clumyap and the command to ~/.local/bin; no sudo needed.
# Uninstall: clumyap uninstall
set -eu

repo="adamwahyuh/clumyap"
share="${XDG_DATA_HOME:-$HOME/.local/share}/clumyap"
bin="$HOME/.local/bin"

# Variables next to non-ASCII text are written as ${name}: in a UTF-8 locale bash reads "$bar█" as a
# variable named "bar" plus the first byte of █, which set -u rejects.
# Colours and animation only on a real terminal; `clumyap update` in the TUI and CI get plain lines.
# CLUMYAP_FANCY=1 forces them on, so CI can run the drawing code too.
if [ "${CLUMYAP_FANCY:-}" = 1 ] || { [ -t 1 ] && [ -z "${NO_COLOR:-}" ] && [ "${TERM:-dumb}" != dumb ]; }; then
	fancy=1
	esc="$(printf '\033')"
	bold="$esc[1m" dim="$esc[2m" green="$esc[32m" red="$esc[31m" purple="$esc[38;5;141m" pink="$esc[38;5;205m" reset="$esc[0m"
else
	fancy=0 esc="" bold="" dim="" green="" red="" purple="" pink="" reset=""
fi

tmp="$(mktemp -d)"
pid=""
cleanup() {
	[ -n "$pid" ] && kill "$pid" 2>/dev/null
	[ "$fancy" = 1 ] && printf '%s[?25h' "$esc" # show the cursor again
	rm -rf "$tmp"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

ok() { printf '  %s✓%s %s\n' "$green" "$reset" "$1"; }
fail() {
	[ "$fancy" = 1 ] && printf '\r%s[K' "$esc"
	printf '  %s✗%s %s\n' "$red" "$reset" "$1" >&2
	exit 1
}

printf '\n  %s%s♪ clumyap%s %sinstaller%s\n\n' "$bold" "$purple" "$reset" "$dim" "$reset"

case "$(uname -s)" in
Darwin) os=macos ;;
Linux) os=linux ;;
*) fail "this installer is for macOS and Linux. On Windows run in PowerShell:
      irm https://raw.githubusercontent.com/$repo/main/install.ps1 | iex" ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "no build for $(uname -m) yet" ;;
esac
# A shell running under Rosetta reports x86_64 on Apple Silicon; the native build is faster.
if [ "$os" = macos ] && [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null)" = 1 ]; then
	arch=arm64
fi
case "$os/$arch" in
macos/arm64) label="macOS (Apple Silicon)" ;;
macos/amd64) label="macOS (Intel)" ;;
linux/amd64) label="Linux (x64)" ;;
linux/arm64) label="Linux (arm64)" ;;
esac
ok "Found $label"

url="${CLUMYAP_URL:-https://github.com/$repo/releases/latest/download/clumyap-$os-$arch.tar.gz}"
file="$tmp/clumyap.tar.gz"

# download draws a spinner, a bar and the size while curl runs in the background.
download() {
	if [ "$fancy" = 0 ]; then
		echo "  Downloading $url"
		curl -fsSL "$url" -o "$file"
		return
	fi
	total="$(curl -fsSLI "$url" 2>/dev/null | tr -d '\r' | awk 'tolower($1) == "content-length:" { n = $2 } END { print n + 0 }')"
	curl -fsSL "$url" -o "$file" &
	pid=$!
	printf '%s[?25l' "$esc" # hide the cursor while drawing
	i=0
	while kill -0 "$pid" 2>/dev/null; do
		case $((i % 10)) in
		0) f=⠋ ;; 1) f=⠙ ;; 2) f=⠹ ;; 3) f=⠸ ;; 4) f=⠼ ;; 5) f=⠴ ;; 6) f=⠦ ;; 7) f=⠧ ;; 8) f=⠇ ;; *) f=⠏ ;;
		esac
		got=0
		[ -f "$file" ] && got="$(wc -c < "$file" | tr -d ' ')"
		if [ "$total" -gt 0 ]; then
			filled=$((got * 20 / total))
			bar="" n=0
			while [ $n -lt 20 ]; do
				if [ $n -lt $filled ]; then bar="${bar}█"; else bar="${bar}░"; fi
				n=$((n + 1))
			done
			printf '\r  %s%s%s Downloading  %s%s%s  %3d%%  %s%d/%d MB%s' "$pink" "$f" "$reset" "$purple" "$bar" "$reset" \
				$((got * 100 / total)) "$dim" $((got / 1048576)) $((total / 1048576)) "$reset"
		else
			printf '\r  %s%s%s Downloading  %s%d MB%s' "$pink" "$f" "$reset" "$dim" $((got / 1048576)) "$reset"
		fi
		i=$((i + 1))
		sleep 0.1
	done
	if wait "$pid"; then status=0; else status=$?; fi
	pid=""
	printf '\r%s[K%s[?25h' "$esc" "$esc"
	return $status
}

download || fail "download failed: $url"
size="$(wc -c < "$file" | tr -d ' ')"
ok "Downloaded clumyap $dim($((size / 1048576)) MB)$reset"

tar -xzf "$file" -C "$tmp" 2>/dev/null || fail "the download is not a clumyap archive"
[ -x "$tmp/clumyap/clumyap" ] || fail "the download is not a clumyap archive"

mkdir -p "$(dirname "$share")" "$bin"
rm -rf "$share"
mv "$tmp/clumyap" "$share"
ln -sf "$share/clumyap" "$bin/clumyap"
# Files fetched by a browser are blocked on macOS until this is cleared; harmless otherwise.
[ "$os" = macos ] && xattr -dr com.apple.quarantine "$share" 2>/dev/null || true

version="$("$bin/clumyap" --version)"
ok "Installed ${bold}${version}${reset} ${dim}→ ${share}${reset}"

# Make sure the command is found in new terminals.
newterm=0
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
	fi
	ok "Added ~/.local/bin to your PATH $dim($rc)$reset"
	newterm=1
	;;
esac

echo
[ "$newterm" = 1 ] && echo "  Open a new terminal window, then:"
printf '  Get started:  %s%sclumyap start%s\n' "$bold" "$purple" "$reset"
printf '  %sThe first run downloads the voice model (about 350 MB).%s\n\n' "$dim" "$reset"
