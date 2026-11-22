#!/usr/bin/env bash
# Builds self-contained clumyap archives for macOS into dist/:
#   dist/clumyap-<version>-macos-<arch>.tar.gz  ->  clumyap-<version>-macos-<arch>/{clumyap, lib/*.dylib, README.md}
# A plain `go install` binary finds its libraries in the Go module cache, which only exists on
# the machine that built it; these archives carry the libraries next to the binary instead.
#
# Usage: scripts/package.sh [arm64|amd64 ...]   (default: both)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
service="$root/service"
version="$(tr -d '[:space:]' < "$service/VERSION")"
archs=("$@")
[ ${#archs[@]} -eq 0 ] && archs=(arm64 amd64)

[ "$(uname -s)" = Darwin ] || { echo "package.sh: macOS only for now" >&2; exit 1; }

moddir() { (cd "$service" && go list -m -f '{{.Dir}}' "$1"); }
sherpa="$(moddir github.com/k2-fsa/sherpa-onnx-go-macos/v2)"
piper="$(moddir github.com/csukuangfj/piper-phonemize-go-macos)"

for arch in "${archs[@]}"; do
	case "$arch" in
	arm64) libarch=aarch64-apple-darwin clangarch=arm64 ;;
	amd64) libarch=x86_64-apple-darwin clangarch=x86_64 ;;
	*) echo "package.sh: unknown arch $arch (use arm64 or amd64)" >&2; exit 2 ;;
	esac
	name="clumyap-$version-macos-$arch"
	out="$root/dist/$name"
	rm -rf "$out" "$out.tar.gz"
	mkdir -p "$out/lib"

	echo "==> $name"
	# -trimpath keeps the builder's home folder and source paths out of the binary.
	(cd "$service" && GOOS=darwin GOARCH="$arch" CGO_ENABLED=1 CC="clang -arch $clangarch" \
		go build -trimpath -ldflags "-s -w" -o "$out/clumyap" .)

	cp "$sherpa/lib/$libarch/libsherpa-onnx-c-api.dylib" "$sherpa/lib/$libarch/libonnxruntime.dylib" \
		"$piper/lib/$libarch/libpiper_phonemize_core.dylib" "$out/lib/"
	chmod 644 "$out"/lib/*.dylib

	# Swap the module-cache rpaths cgo baked in for one next to the binary.
	otool -l "$out/clumyap" | awk '/LC_RPATH/{getline; getline; print $2}' | while read -r rp; do
		install_name_tool -delete_rpath "$rp" "$out/clumyap"
	done
	install_name_tool -add_rpath @executable_path/lib "$out/clumyap"

	# Editing a Mach-O breaks its signature, and Apple Silicon refuses to run unsigned code: re-sign ad hoc.
	codesign --force --sign - "$out"/lib/*.dylib "$out/clumyap" 2>/dev/null

	cp "$root/README.md" "$out/"
	[ -f "$root/LICENSE" ] && cp "$root/LICENSE" "$out/"

	# Guard against shipping something that only works here.
	if otool -l "$out/clumyap" | awk '/LC_RPATH/{getline; getline; print $2}' | grep -qv '^@executable_path/lib$'; then
		echo "package.sh: unexpected rpath left in $name" >&2; exit 1
	fi
	if strings "$out/clumyap" | grep -q "$HOME"; then
		echo "package.sh: $name still contains $HOME" >&2; exit 1
	fi

	tar -C "$root/dist" -czf "$out.tar.gz" "$name"
	echo "    $(du -h "$out.tar.gz" | cut -f1)  dist/$name.tar.gz"
done
