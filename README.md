<p align="center"><img src="assets/logo/icon.png" width="160" alt="Clumyap logo"></p>

# Clumyap

Offline English text-to-speech. Kokoro voices running locally through sherpa-onnx, output as MP3 or WAV, with no account and no internet after the first model download.

## Repository layout

| Folder | What it holds |
|---|---|
| `service/` | The core in Go: TTS engine, `clumyap` CLI and the `clumyap start` interactive terminal UI |
| `assets/logo/` | App icon: `icon.svg` (source) and `icon.png` (1024 px, transparent) |

Clients for other platforms (Android, desktop) go in their own top-level folders next to `service/`.

## Build and install

Needs Go 1.25 or newer, on macOS (Apple Silicon or Intel), Linux or Windows.

```bash
cd service
go install .        # puts the clumyap binary in ~/go/bin
go test ./...       # runs the tests; a few skip until the model is downloaded
```

The first run downloads the Kokoro model (about 350 MB) to `~/.cache/clumyap`.

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

## Credits

Made by Adam Wahyu Hariyadi. `/info` inside `clumyap start` shows the same list.

| Part | By | License | Used for |
|---|---|---|---|
| Kokoro-82M | hexgrad | Apache-2.0 | the voices |
| sherpa-onnx | k2-fsa | Apache-2.0 | runs the voice model |
| ONNX Runtime | Microsoft | MIT | neural network engine |
| misaki | hexgrad | Apache-2.0 | espeak-to-Kokoro sound rules |
| espeak-ng | espeak-ng team | GPL-3.0 | words missing from Kokoro's lexicon |
| piper-phonemize | rhasspy | MIT | connects espeak-ng |
| shine-mp3 | braheezy | LGPL-2.0 | MP3 encoder |
| Bubble Tea, Bubbles, Lip Gloss | Charm | MIT | terminal UI |
