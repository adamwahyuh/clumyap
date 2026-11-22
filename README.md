<p align="center"><img src="assets/logo/icon.png" width="160" alt="Clumyap logo"></p>

# Clumyap

Offline English text-to-speech. Kokoro voices running locally through sherpa-onnx, output as MP3 or WAV, with no account and no internet after the first model download.

## Repository layout

| Folder | What it holds |
|---|---|
| `service/` | The core in Go: TTS engine, `clumyap` CLI and the `clumyap start` interactive terminal UI |
| `assets/logo/` | App icon: `icon.svg` (source) and `icon.png` (1024 px, transparent) |

Clients for other platforms (Android, desktop) go in their own top-level folders next to `service/`.

## Install from a release archive (macOS)

Download `clumyap-<version>-macos-arm64.tar.gz` (Apple Silicon) or `-amd64` (Intel), then:

```bash
tar -xzf clumyap-0.1.0-macos-arm64.tar.gz
mkdir -p ~/.local/share && mv clumyap-0.1.0-macos-arm64 ~/.local/share/clumyap
xattr -dr com.apple.quarantine ~/.local/share/clumyap   # files from the internet are blocked until this
ln -sf ~/.local/share/clumyap/clumyap /usr/local/bin/clumyap
clumyap --version
```

Keep the `lib/` folder next to the `clumyap` binary; the binary loads its libraries from there. The archives are signed ad hoc, not notarized by Apple, which is why the `xattr` step is needed.

## Build and install

Needs Go 1.25 or newer, on macOS (Apple Silicon or Intel), Linux or Windows.

```bash
cd service
go install .        # puts the clumyap binary in ~/go/bin
go test ./...       # runs the tests; a few skip until the model is downloaded
```

The first run downloads the Kokoro model (about 350 MB) to `~/.cache/clumyap`.

A `go install` build finds its libraries in your Go module cache, so it only runs on the machine that built it. To make archives you can give to others:

```bash
scripts/package.sh            # dist/clumyap-<version>-macos-arm64.tar.gz and -amd64.tar.gz
```

The version lives in `service/VERSION` and is compiled into the binary; `clumyap --version` prints it. Bump it there and rebuild.

## Use

```bash
clumyap "Hello there. How are you?"           # writes clumyap-<time>.mp3
clumyap -f story.txt -o story.mp3 -v bella    # read a file with a chosen voice
clumyap start                                  # interactive session, /help inside
clumyap --voices                               # list voices
clumyap --set voice=bella --set sentence-pause=0.4   # save defaults
clumyap --help                                 # every flag
```

On macOS, `--play` and the interactive session play audio with the built-in `afplay`.

### Teaching pronunciation

Names and new words can be taught in `~/.config/clumyap/lexicon.txt`, one `word = say as` per line, or with `/pronounce` inside `clumyap start`:

```
clumyap = clum yap        # a respelling in ordinary words
adam = /ˈædəm/            # or Kokoro phonemes between slashes
```

Entries apply to the CLI and the interactive session, and are read fresh on every run.

Common chat shorthand is read in full without any setup: btw, idk, imo, tbh, brb, pls, thx, otw, hbd, nvm and more (`chatWords` in `service/g2p.go`). Shorthand usually said as letters (omg, lol, fyi, asap) stays as it is.

## License

Copyright (C) 2026 Adam Wahyu Hariyadi

Clumyap is free software: you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version. It is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See [LICENSE](LICENSE) for the full text.

It is GPL because it links espeak-ng (GPL-3.0); every other part below is under a license compatible with it.

## Credits

Made by Adam Wahyu Hariyadi. `/info` inside `clumyap start` shows the same list.

| Part | By | License | Used for |
|---|---|---|---|
| Kokoro-82M | hexgrad | Apache-2.0 | the voices |
| sherpa-onnx | k2-fsa | Apache-2.0 | runs the voice model |
| ONNX Runtime | Microsoft | MIT | neural network engine |
| misaki | hexgrad | Apache-2.0 | sound rules and heteronym list |
| espeak-ng | espeak-ng team | GPL-3.0 | words missing from Kokoro's lexicon |
| piper-phonemize | rhasspy | MIT | connects espeak-ng |
| shine-mp3 | braheezy | LGPL-2.0 | MP3 encoder |
| Bubble Tea, Bubbles, Lip Gloss | Charm | MIT | terminal UI |
