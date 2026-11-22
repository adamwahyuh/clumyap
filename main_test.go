package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	o, err := parseArgs([]string{"Hello", "world", "-o", "a.wav", "-v", "am_michael", "--speed", "1.2"})
	if err != nil || o.text != "Hello world" || o.output != "a.wav" || o.voice != "am_michael" || o.speed != 1.2 {
		t.Fatalf("got %+v, %v", o, err)
	}
	if voiceID("Sarah") != voiceID("af_sarah") || voiceID("af_sarah") != 9 || voiceID("bm_lewis") != 27 || voiceID("x") != -1 {
		t.Error("voiceID lookup wrong")
	}
	t.Setenv("CLUMYAP_VOICE", "bella")
	if o, err := parseArgs([]string{"hi"}); err != nil || o.voice != "bella" {
		t.Errorf("CLUMYAP_VOICE not applied: %q %v", o.voice, err)
	}
	t.Setenv("CLUMYAP_OUTPUT_DIR", "/tmp/out")
	if o, _ := parseArgs([]string{"hi"}); !strings.HasPrefix(o.output, "/tmp/out/clumyap-") {
		t.Errorf("output dir not applied: %s", o.output)
	}
	if o, _ := parseArgs([]string{"hi", "-o", "x.mp3"}); o.output != "x.mp3" {
		t.Errorf("-o must win over CLUMYAP_OUTPUT_DIR: %s", o.output)
	}
	t.Setenv("CLUMYAP_MODEL", "kokoro-int8-multi-lang-v1_0")
	if o, _ := parseArgs([]string{"hi"}); o.model != "kokoro-int8-multi-lang-v1_0" {
		t.Errorf("CLUMYAP_MODEL not applied: %s", o.model)
	}
	if o, _ := parseArgs([]string{"hi", "-m", "kokoro-multi-lang-v1_0"}); o.model != "kokoro-multi-lang-v1_0" {
		t.Errorf("--model must win over CLUMYAP_MODEL: %s", o.model)
	}
	for _, bad := range [][]string{
		{"hi", "-s", "3"},
		{"hi", "-v", "nobody"},
		{"hi", "-o", "a.ogg"},
		{"hi", "-f", "x.txt"},
		{"hi", "-o", "-", "--play"},
		{"hi", "--nope"},
		{"hi", "-m", "kokoro-v9"},
	} {
		if _, err := parseArgs(bad); !errors.Is(err, errUsage) {
			t.Errorf("%v: want usage error, got %v", bad, err)
		}
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
