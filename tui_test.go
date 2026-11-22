package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTUICommands(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	o, err := parseArgs(nil, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	m := newTUI(o)

	m.command("/voice George")
	m.command("/speed 1.25")
	m.command("/lang en-gb")
	if m.o.voice != "bm_george" || m.o.speed != 1.25 || m.o.lang != "en-gb" {
		t.Fatalf("settings not applied: %+v", m.o)
	}
	m.command("/voice nobody")
	m.command("/speed 7")
	if m.o.voice != "bm_george" || m.o.speed != 1.25 {
		t.Fatalf("invalid values must not change settings: %+v", m.o)
	}
	for _, c := range []string{"/save", "/replay", "/bogus", "/help", "/voice", "/model", "/speed"} {
		if m.command(c) == nil {
			t.Errorf("%s: want a message", c)
		}
	}

	m.command("/default")
	cfg, _ := loadConfig()
	if cfg["voice"] != "bm_george" || cfg["speed"] != "1.25" || cfg["lang"] != "en-gb" {
		t.Fatalf("/default not saved: %v", cfg)
	}

	// /save needs audio; fake some and an engine rate without loading a model.
	m.last, m.eng = make([]float32, 2400), &engine{rate: 24000}
	out := filepath.Join(t.TempDir(), "a", "b.wav")
	m.command("/save " + out)
	if st, err := os.Stat(out); err != nil || st.Size() != 44+4800 {
		t.Fatalf("/save: %v", err)
	}
}
