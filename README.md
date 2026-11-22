<p align="center"><img src="assets/logo/icon.png" width="160" alt="Clumyap logo"></p>

# Clumyap

Offline English text-to-speech. Kokoro voices running locally through sherpa-onnx, output as MP3 or WAV, with no account and no internet after the first model download.

## Repository layout

| Folder | What it holds |
|---|---|
| `service/` | The core in Go: TTS engine, `clumyap` CLI and the `clumyap start` interactive terminal UI |
| `assets/logo/` | App icon: `icon.svg` (source) and `icon.png` (1024 px, transparent) |

Clients for other platforms (Android, desktop) go in their own top-level folders next to `service/`.

## Install

**macOS and Linux**, in a terminal:

```bash
curl -fsSL https://raw.githubusercontent.com/adamwahyuh/clumyap/main/install.sh | sh
```

**Windows**, in PowerShell:

```powershell
irm https://raw.githubusercontent.com/adamwahyuh/clumyap/main/install.ps1 | iex
```

Then open a new terminal and run `clumyap start`. The first run downloads the voice model (about 350 MB). Running the same command again updates clumyap. No admin rights are needed: files go to `~/.local/share/clumyap` (Windows: `%LOCALAPPDATA%\Programs\clumyap`).

Builds exist for macOS (Apple Silicon, Intel), Linux (x64, arm64) and Windows (x64). Audio playback uses `afplay` on macOS, Windows' built-in player, and `pw-play`, `paplay`, `aplay` or `ffplay` on Linux.

To uninstall: `rm -rf ~/.local/share/clumyap ~/.local/bin/clumyap` (Windows: delete the folder above and remove it from your user PATH).

## Build and install

Needs Go 1.25 or newer, on macOS (Apple Silicon or Intel), Linux or Windows.

```bash
cd service
go install .        # puts the clumyap binary in ~/go/bin
go test ./...       # runs the tests; a few skip until the model is downloaded
```

The first run downloads the Kokoro model (about 350 MB) to `~/.cache/clumyap`.

A `go install` build finds its libraries in your Go module cache, so it only runs on the machine that built it. `scripts/package.sh` makes self-contained archives for the OS it runs on (`dist/clumyap-<os>-<arch>.tar.gz`, `.zip` on Windows).

Releases are built by GitHub Actions (`.github/workflows/release.yml`) on real macOS, Linux and Windows machines. To publish one, set the version in `service/VERSION`, commit, then push a matching tag:

```bash
git tag v0.1.0 && git push origin v0.1.0
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
