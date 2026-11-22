package main

import (
	"fmt"
	"path/filepath"
	"runtime"

	pp "github.com/csukuangfj/piper-phonemize-go/piper_phonemize"
	sherpa "github.com/k2-fsa/sherpa-onnx-go/v2/sherpa_onnx"
)

// engine is the loaded TTS core, shared by the one-shot CLI and `clumyap start`. Not safe for concurrent use.
type engine struct {
	tts   *sherpa.OfflineTts
	model model
	rate  int
	dir   string
	g2ps  map[string]*g2p // per lang, loaded on first use
}

// loadEngine loads an installed model; call ensureModel first.
func loadEngine(m model) (*engine, error) {
	base, err := modelDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, m.name)
	cfg := sherpa.OfflineTtsConfig{MaxNumSentences: 1}
	cfg.Model.Kokoro = sherpa.OfflineTtsKokoroModelConfig{
		Model:       filepath.Join(dir, m.file),
		Voices:      filepath.Join(dir, "voices.bin"),
		Tokens:      filepath.Join(dir, "tokens.txt"),
		LengthScale: 1.0,
	}
	cfg.Model.NumThreads = min(runtime.NumCPU(), 8)
	tts := sherpa.NewOfflineTts(&cfg)
	if tts == nil {
		return nil, fmt.Errorf("failed to load model from %s (delete that folder to re-download)", dir)
	}
	// espeak covers words the lexicon lacks (sherpa's own lexicon path silently drops them).
	// Initialize only takes effect once per process; every model ships the same espeak-ng-data.
	if pp.Initialize(filepath.Join(dir, "espeak-ng-data")) < 0 {
		sherpa.DeleteOfflineTts(tts)
		return nil, fmt.Errorf("failed to load espeak-ng data from %s", dir)
	}
	return &engine{tts: tts, model: m, rate: tts.SampleRate(), dir: dir, g2ps: map[string]*g2p{}}, nil
}

func (e *engine) close() { sherpa.DeleteOfflineTts(e.tts) }

// trimSilence cuts near-silent samples from both ends, keeping 40 ms so word onsets and tails are not clipped.
func trimSilence(s []float32, rate int) []float32 {
	const threshold = 0.01
	keep := rate * 40 / 1000
	start, end := 0, len(s)
	for start < end && s[start] < threshold && s[start] > -threshold {
		start++
	}
	for end > start && s[end-1] < threshold && s[end-1] > -threshold {
		end--
	}
	return s[max(0, start-keep):min(len(s), end+keep)]
}

// synth speaks text with o's voice, lang, speed, pause (between paragraphs) and sentencePause,
// returning mono float samples at e.rate. progress may be nil.
func (e *engine) synth(text string, o options, progress func(para, total int, pct float32)) ([]float32, error) {
	paras := paragraphs(text)
	if len(paras) == 0 {
		return nil, fmt.Errorf("%w: text is empty", errUsage)
	}
	g, ok := e.g2ps[o.lang]
	if !ok {
		var err error
		if g, err = newG2P(e.dir, o.lang); err != nil {
			return nil, fmt.Errorf("load lexicon: %w", err)
		}
		e.g2ps[o.lang] = g
	}
	g.user, _ = loadUserLexicon() // read every time so edits apply at once; a missing file is no entries
	paraSilence := make([]float32, int(float64(e.rate)*o.pause))
	sentSilence := make([]float32, int(float64(e.rate)*o.sentencePause))
	var samples []float32
	for i, p := range paras {
		// By default a paragraph is one model call so intonation flows across sentences;
		// with a sentence pause each sentence is its own call so silence can go between them.
		units := [][]string{g.chunks(p)}
		if o.sentencePause > 0 {
			units = nil
			for _, s := range g.sentences(p) {
				units = append(units, pack([]string{s}))
			}
		}
		for j, unit := range units {
			if len(unit) == 0 {
				continue
			}
			var codepoints [][]int32
			for _, c := range unit {
				codepoints = append(codepoints, []int32(c))
			}
			gen := &sherpa.GenerationConfig{Sid: voiceID(o.voice), Speed: float32(o.speed), PhonemeCodepoints: codepoints}
			audio := e.tts.GenerateWithConfig(p, gen, func(_ []float32, pct float32) bool {
				if progress != nil {
					progress(i+1, len(paras), (float32(j)+pct)/float32(len(units)))
				}
				return true
			})
			if audio == nil {
				return nil, fmt.Errorf("speech generation failed on paragraph %d", i+1)
			}
			out := audio.Samples
			if o.sentencePause > 0 {
				// Each call pads its own silence at both ends; trim it so the gap is what was asked for.
				out = trimSilence(out, e.rate)
			}
			switch {
			case len(samples) == 0:
			case j == 0:
				samples = append(samples, paraSilence...)
			default:
				samples = append(samples, sentSilence...)
			}
			samples = append(samples, out...)
		}
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("%w: nothing speakable in the text", errUsage)
	}
	return normalize(samples, e.rate), nil
}
