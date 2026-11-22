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
	// The lexicon alone silently drops unknown words, numbers and inflections; espeak phonemizes everything.
	// Initialize only takes effect once per process; every model ships the same espeak-ng-data.
	if pp.Initialize(filepath.Join(dir, "espeak-ng-data")) < 0 {
		sherpa.DeleteOfflineTts(tts)
		return nil, fmt.Errorf("failed to load espeak-ng data from %s", dir)
	}
	return &engine{tts: tts, model: m, rate: tts.SampleRate()}, nil
}

func (e *engine) close() { sherpa.DeleteOfflineTts(e.tts) }

// synth speaks text (split into paragraphs) and returns mono float samples at e.rate.
// progress may be nil.
func (e *engine) synth(text, voice, lang string, speed float64, progress func(para, total int, pct float32)) ([]float32, error) {
	paras := paragraphs(text)
	if len(paras) == 0 {
		return nil, fmt.Errorf("%w: text is empty", errUsage)
	}
	espeakVoice := map[string]string{"en-us": "en-us", "en-gb": "en-gb-x-rp"}[lang]
	pause := make([]float32, e.rate*6/10) // 0.6 s between paragraphs
	var samples []float32
	for i, p := range paras {
		gen := &sherpa.GenerationConfig{Sid: voiceID(voice), Speed: float32(speed), PhonemeCodepoints: phonemize(p, espeakVoice)}
		audio := e.tts.GenerateWithConfig(p, gen, func(_ []float32, pct float32) bool {
			if progress != nil {
				progress(i+1, len(paras), pct)
			}
			return true
		})
		if audio == nil {
			return nil, fmt.Errorf("speech generation failed on paragraph %d", i+1)
		}
		if i > 0 {
			samples = append(samples, pause...)
		}
		samples = append(samples, audio.Samples...)
	}
	return samples, nil
}

func phonemize(text, voice string) [][]int32 {
	r := pp.Phonemize(text, voice)
	defer pp.DeletePhonemizeResult(r)
	out := make([][]int32, r.GetNumSentences())
	for s := range out {
		for _, c := range r.GetPhonemes(s) {
			out[s] = append(out[s], int32(c))
		}
	}
	return out
}
