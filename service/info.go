package main

import (
	"fmt"
	"strings"
)

const (
	appAuthor = "Adam Wahyu Hariyadi"
	appEmail  = "adamdesign19@gmail.com"
)

// credits lists what clumyap is built on; keep it in step with go.mod and the models offered.
var credits = []struct{ name, by, license, role string }{
	{"Kokoro-82M", "hexgrad", "Apache-2.0", "the voices"},
	{"sherpa-onnx", "k2-fsa", "Apache-2.0", "runs the voice model"},
	{"ONNX Runtime", "Microsoft", "MIT", "neural network engine"},
	{"misaki", "hexgrad", "Apache-2.0", "espeak-to-Kokoro sound rules"},
	{"espeak-ng", "espeak-ng team", "GPL-3.0", "words missing from the lexicon"},
	{"piper-phonemize", "rhasspy", "MIT", "connects espeak-ng"},
	{"shine-mp3", "braheezy", "LGPL-2.0", "MP3 encoder"},
	{"Bubble Tea", "Charm", "MIT", "terminal UI (+ Bubbles, Lip Gloss)"},
}

func infoText() string {
	var b strings.Builder
	b.WriteString(accent.Render("clumyap") + dim.Render(" v"+version()+" · offline English text-to-speech") + "\n\n")
	b.WriteString(dim.Render("made by  ") + pink.Render(appAuthor) + "\n")
	b.WriteString(dim.Render("email    ") + cyan.Render(appEmail) + "\n\n")
	b.WriteString(accent.Render("built with") + "\n")
	for _, c := range credits {
		b.WriteString("  " + fmt.Sprintf("%-16s", c.name) + dim.Render(fmt.Sprintf("%-16s", c.by)) +
			amber.Render(fmt.Sprintf("%-12s", c.license)) + dim.Render(c.role) + "\n")
	}
	base, _ := modelDir()
	cfg, _ := configPath()
	b.WriteString("\n" + dim.Render("models   ") + tildePath(base) + "\n")
	b.WriteString(dim.Render("config   ") + tildePath(cfg))
	return b.String()
}
