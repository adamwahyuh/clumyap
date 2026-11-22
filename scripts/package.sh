#!/usr/bin/env bash
# Builds self-contained clumyap archives for the OS it runs on, into dist/:
#   macOS    dist/clumyap-macos-<arch>.tar.gz     clumyap/{clumyap, lib/*.dylib}
#   Linux    dist/clumyap-linux-<arch>.tar.gz     clumyap/{clumyap, lib/*.so}
#   Windows  dist/clumyap-windows-<arch>.zip      clumyap/{clumyap.exe, *.dll}
# each with README.md and LICENSE. The names carry no version so the installers can always fetch
# GitHub's "latest" release; `clumyap --version` tells the version.
#
# A plain `go install` binary finds its libraries in the Go module cache, which only exists on the
# machine that built it; these archives carry the libraries next to the binary instead.
#
# Usage: scripts/package.sh [arm64|amd64 ...]
#   macOS builds both by default (Intel is cross-compiled); Linux and Windows build the host arch.
# Linux needs patchelf.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
service="$root/service"
goos="$(cd "$service" && go env GOOS)"
archs=("$@")
if [ ${#archs[@]} -eq 0 ]; then
	if [ "$goos" = darwin ]; then archs=(arm64 amd64); else archs=("$(cd "$service" && go env GOARCH)"); fi
fi
moddir() { (cd "$service" && go list -m -f '{{.Dir}}' "$1"); }

for arch in "${archs[@]}"; do
	case "$goos/$arch" in
	darwin/arm64) os=macos libarch=aarch64-apple-darwin ;;
	darwin/amd64) os=macos libarch=x86_64-apple-darwin ;;
	linux/amd64) os=linux libarch=x86_64-unknown-linux-gnu ;;
	linux/arm64) os=linux libarch=aarch64-unknown-linux-gnu ;;
	windows/amd64) os=windows libarch=x86_64-pc-windows-gnu ;;
	*) echo "package.sh: $goos/$arch is not supported" >&2; exit 2 ;;
	esac
	name="clumyap-$os-$arch"
	stage="$root/dist/$name"
	out="$stage/clumyap"
	rm -rf "$stage" "$root/dist/$name.tar.gz" "$root/dist/$name.zip"
	mkdir -p "$out"
	echo "==> $name"

	exe=clumyap
	[ "$os" = windows ] && exe=clumyap.exe
	cc=()
	[ "$os" = macos ] && cc=(CC="clang -arch $([ "$arch" = arm64 ] && echo arm64 || echo x86_64)")
	# -trimpath keeps the builder's home folder and source paths out of the binary.
	(cd "$service" && env GOOS="$goos" GOARCH="$arch" CGO_ENABLED=1 "${cc[@]}" \
		go build -trimpath -ldflags "-s -w" -o "$out/$exe" .)

	case "$os" in
	macos) sherpa="$(moddir github.com/k2-fsa/sherpa-onnx-go-macos/v2)" piper="$(moddir github.com/csukuangfj/piper-phonemize-go-macos)" ;;
	linux) sherpa="$(moddir github.com/k2-fsa/sherpa-onnx-go-linux/v2)" piper="$(moddir github.com/csukuangfj/piper-phonemize-go-linux)" ;;
	windows) sherpa="$(moddir github.com/k2-fsa/sherpa-onnx-go-windows/v2)" piper="$(moddir github.com/csukuangfj/piper-phonemize-go-windows)" ;;
	esac

	case "$os" in
	macos)
		mkdir -p "$out/lib"
		cp "$sherpa/lib/$libarch/libsherpa-onnx-c-api.dylib" "$sherpa/lib/$libarch/libonnxruntime.dylib" \
			"$piper/lib/$libarch/libpiper_phonemize_core.dylib" "$out/lib/"
		chmod 644 "$out"/lib/*
		# Swap the module-cache rpaths cgo baked in for one next to the binary.
		otool -l "$out/$exe" | awk '/LC_RPATH/{getline; getline; print $2}' | while read -r rp; do
			install_name_tool -delete_rpath "$rp" "$out/$exe"
		done
		install_name_tool -add_rpath @executable_path/lib "$out/$exe"
		# Editing a Mach-O breaks its signature, and Apple Silicon refuses unsigned code: re-sign ad hoc.
		codesign --force --sign - "$out"/lib/*.dylib "$out/$exe" 2>/dev/null
		rpaths="$(otool -l "$out/$exe" | awk '/LC_RPATH/{getline; getline; print $2}')"
		[ "$rpaths" = "@executable_path/lib" ] || { echo "package.sh: unexpected rpaths: $rpaths" >&2; exit 1; }
		;;
	linux)
		mkdir -p "$out/lib"
		cp "$sherpa/lib/$libarch/libsherpa-onnx-c-api.so" "$sherpa/lib/$libarch/libonnxruntime.so" \
			"$piper/lib/$libarch/libpiper_phonemize_core.so" "$out/lib/"
		chmod 644 "$out"/lib/*
		patchelf --set-rpath '$ORIGIN/lib' "$out/$exe"
		rpaths="$(patchelf --print-rpath "$out/$exe")"
		[ "$rpaths" = '$ORIGIN/lib' ] || { echo "package.sh: unexpected rpath: $rpaths" >&2; exit 1; }
		;;
	windows)
		# Windows looks for DLLs next to the .exe first, so they sit beside it.
		cp "$sherpa/lib/$libarch/sherpa-onnx-c-api.dll" "$sherpa/lib/$libarch/onnxruntime.dll" \
			"$piper/lib/$libarch/piper_phonemize_core.dll" "$out/"
		;;
	esac

	if grep -qaF "$HOME" "$out/$exe"; then
		echo "package.sh: $name still contains $HOME" >&2; exit 1
	fi
	cp "$root/README.md" "$root/LICENSE" "$out/"

	if [ "$os" = windows ]; then
		powershell -NoProfile -Command "Compress-Archive -Path '$(cygpath -w "$out")' -DestinationPath '$(cygpath -w "$root/dist/$name.zip")'"
		echo "    dist/$name.zip"
	else
		tar -C "$stage" -czf "$root/dist/$name.tar.gz" clumyap
		echo "    $(du -h "$root/dist/$name.tar.gz" | cut -f1)  dist/$name.tar.gz"
	fi
	rm -rf "$stage"
done
