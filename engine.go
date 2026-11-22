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

// synth speaks text (split into paragraphs, pause seconds of silence between them) and returns
// mono float samples at e.rate. progress may be nil.
func (e *engine) synth(text, voice, lang string, speed, pause float64, progress func(para, total int, pct float32)) ([]float32, error) {
	paras := paragraphs(text)
	if len(paras) == 0 {
		return nil, fmt.Errorf("%w: text is empty", errUsage)
	}
	g, ok := e.g2ps[lang]
	if !ok {
		var err error
		if g, err = newG2P(e.dir, lang); err != nil {
			return nil, fmt.Errorf("load lexicon: %w", err)
		}
		e.g2ps[lang] = g
	}
	silence := make([]float32, int(float64(e.rate)*pause))
	var samples []float32
	for i, p := range paras {
		var codepoints [][]int32
		for _, c := range g.chunks(p) {
			codepoints = append(codepoints, []int32(c))
		}
		if len(codepoints) == 0 {
			continue
		}
		gen := &sherpa.GenerationConfig{Sid: voiceID(voice), Speed: float32(speed), PhonemeCodepoints: codepoints}
		audio := e.tts.GenerateWithConfig(p, gen, func(_ []float32, pct float32) bool {
			if progress != nil {
				progress(i+1, len(paras), pct)
			}
			return true
		})
		if audio == nil {
			return nil, fmt.Errorf("speech generation failed on paragraph %d", i+1)
		}
		if len(samples) > 0 {
			samples = append(samples, silence...)
		}
		samples = append(samples, audio.Samples...)
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("%w: nothing speakable in the text", errUsage)
	}
	return samples, nil
}
