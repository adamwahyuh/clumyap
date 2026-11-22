package main

import (
	"bytes"
	"errors"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	o, err := parseArgs([]string{"Hello", "world", "-o", "a.wav", "-v", "am_michael", "--speed", "1.2"}, nil)
	if err != nil || o.text != "Hello world" || o.output != "a.wav" || o.voice != "am_michael" || o.speed != 1.2 {
		t.Fatalf("got %+v, %v", o, err)
	}
	if voiceID("Sarah") != voiceID("af_sarah") || voiceID("af_sarah") != 9 || voiceID("bm_lewis") != 27 || voiceID("x") != -1 {
		t.Error("voiceID lookup wrong")
	}

	cfg := map[string]string{"voice": "af_bella", "speed": "1.3", "output-dir": "/tmp/out", "model": "kokoro-int8-multi-lang-v1_0"}
	o, err = parseArgs([]string{"hi"}, cfg)
	if err != nil || o.voice != "af_bella" || o.speed != 1.3 || o.model != cfg["model"] || !strings.HasPrefix(o.output, "/tmp/out/clumyap-") {
		t.Errorf("config not applied: %+v %v", o, err)
	}
	o, _ = parseArgs([]string{"hi", "-o", "x.mp3", "-v", "adam", "-s", "1", "-m", "kokoro-multi-lang-v1_0"}, cfg)
	if o.output != "x.mp3" || o.voice != "adam" || o.speed != 1 || o.model != "kokoro-multi-lang-v1_0" {
		t.Errorf("flags must win over config: %+v", o)
	}
	for _, bad := range [][]string{
		{"hi", "-s", "3"},
		{"hi", "-v", "nobody"},
		{"hi", "-o", "a.ogg"},
		{"hi", "-f", "x.txt"},
		{"hi", "-o", "-", "--play"},
		{"hi", "--nope"},
		{"hi", "-m", "kokoro-v9"},
		{"hi", "--sentence-pause", "5"},
	} {
		if _, err := parseArgs(bad, nil); !errors.Is(err, errUsage) {
			t.Errorf("%v: want usage error, got %v", bad, err)
		}
	}
}

func TestConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg, err := loadConfig()
	if err != nil || len(cfg) != 0 {
		t.Fatalf("missing file should mean empty config: %v %v", cfg, err)
	}
	if err := updateConfig(cfg, []string{"voice=Bella", "output-dir=~/Music/x", "speed=1.10"}, nil); err != nil {
		t.Fatal(err)
	}
	cfg, _ = loadConfig()
	want := map[string]string{"voice": "af_bella", "output-dir": home + "/Music/x", "speed": "1.1"}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("got %v", cfg)
	}
	if err := updateConfig(cfg, nil, []string{"voice"}); err != nil {
		t.Fatal(err)
	}
	if cfg, _ = loadConfig(); cfg["voice"] != "" || cfg["speed"] != "1.1" {
		t.Fatalf("unset failed: %v", cfg)
	}
	for _, bad := range [][]string{{"voice=nobody"}, {"speed=9"}, {"color=red"}, {"voice"}, {"output-dir="}} {
		if err := updateConfig(map[string]string{}, bad, nil); !errors.Is(err, errUsage) {
			t.Errorf("%v: want usage error, got %v", bad, err)
		}
	}
	if err := updateConfig(map[string]string{}, nil, []string{"color"}); !errors.Is(err, errUsage) {
		t.Errorf("unset unknown: want usage error, got %v", err)
	}
	// A hand-edited bad value is dropped on load instead of breaking every run.
	path, _ := configPath()
	os.WriteFile(path, []byte(`{"speed":"fast","voice":"af_sky"}`), 0o644)
	if cfg, err = loadConfig(); err != nil || !reflect.DeepEqual(cfg, map[string]string{"voice": "af_sky"}) {
		t.Fatalf("got %v %v", cfg, err)
	}
}

func TestParagraphs(t *testing.T) {
	got := paragraphs("One.\nStill one.\n\n  \n\nTwo.  \n\n")
	if want := []string{"One. Still one.", "Two."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestEncoders(t *testing.T) {
	samples := make([]float32, 24000) // 1 s of silence, not a multiple of the MP3 frame size
	samples[0] = 2                    // out of range must clip, not wrap
	var w, m bytes.Buffer
	if err := writeWAV(&w, samples, 24000); err != nil || w.Len() != 44+48000 || string(w.Bytes()[:4]) != "RIFF" {
		t.Fatalf("wav: len %d, %v", w.Len(), err)
	}
	if toPCM16(samples)[0] != 32767 {
		t.Fatal("no clipping")
	}
	if err := writeMP3(&m, samples, 24000); err != nil || m.Len() == 0 || m.Bytes()[0] != 0xFF {
		t.Fatalf("mp3: len %d, %v", m.Len(), err)
	}
}

func TestNormalize(t *testing.T) {
	level := func(s []float32) float64 { // RMS in dBFS
		var sum float64
		for _, x := range s {
			sum += float64(x) * float64(x)
		}
		return 20 * math.Log10(math.Sqrt(sum/float64(len(s))))
	}
	sine := func(amp float64, n int) []float32 {
		s := make([]float32, n)
		for i := range s {
			s[i] = float32(amp * math.Sin(float64(i)/5))
		}
		return s
	}
	// A quiet and a loud signal both end at -20 dBFS.
	for _, amp := range []float64{0.05, 0.3} {
		if got := level(normalize(sine(amp, 24000), 24000)); math.Abs(got+20) > 0.5 {
			t.Errorf("amp %.2f: level %.1f dBFS, want -20", amp, got)
		}
	}
	// Long silence around the speech does not change the gain.
	padded := append(append(make([]float32, 48000), sine(0.05, 24000)...), make([]float32, 48000)...)
	if got := level(normalize(padded, 24000)[48000:72000]); math.Abs(got+20) > 0.5 {
		t.Errorf("speech between pauses: %.1f dBFS, want -20", got)
	}
	// Peaks never pass full scale, silence stays silent, and a whisper is not boosted more than 12 dB.
	for _, x := range normalize([]float32{0.05, 0.9, -0.95, 0.001}, 24000) {
		if math.Abs(float64(x)) > 1 {
			t.Fatalf("sample %v over full scale", x)
		}
	}
	if out := normalize(make([]float32, 1000), 24000); out[0] != 0 {
		t.Fatal("silence must stay silent")
	}
	if got := level(normalize(sine(0.005, 24000), 24000)); got > level(sine(0.005, 24000))+12.1 {
		t.Errorf("gain above +12 dB: %.1f dBFS", got)
	}
}

func TestUpdateHelpers(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"0.2.0", "0.1.0", true}, {"0.10.0", "0.9.3", true}, {"1.0", "0.99.99", true}, {"0.1.0", "0.1.0", false}, {"0.1.0", "0.2.0", false}, {"0.1.1", "0.1", true}} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
	// The test binary lives in a temp build folder, not in the install folder.
	if installedByScript() {
		t.Error("a test binary should not count as installed by the script")
	}
	if _, err := checkUpdate(); !errors.Is(err, errFromSource) {
		t.Errorf("a build from source must refuse to update, got %v", err)
	}
}
