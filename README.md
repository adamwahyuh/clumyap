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

## Licenses of the parts it uses

- Kokoro model: Apache 2.0
- sherpa-onnx: Apache 2.0
- espeak-ng (pronounces words missing from Kokoro's lexicon): GPL-3.0
