package main

import (
	"os"
	"path/filepath"
	"strings"
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
	m.command("/sentence-pause 0.4")
	if m.o.voice != "bm_george" || m.o.speed != 1.25 || m.o.lang != "en-gb" || m.o.sentencePause != 0.4 {
		t.Fatalf("settings not applied: %+v", m.o)
	}
	m.command("/voice nobody")
	m.command("/speed 7")
	if m.o.voice != "bm_george" || m.o.speed != 1.25 {
		t.Fatalf("invalid values must not change settings: %+v", m.o)
	}
	for _, c := range []string{"/save", "/replay", "/bogus", "/help", "/voice", "/model", "/speed", "/config"} {
		if m.command(c) == nil {
			t.Errorf("%s: want a message", c)
		}
	}

	if txt := configText(m.o.cfg, func(s string) string { return "[" + s + "]" }); !strings.Contains(txt, "voice           af_heart (default)") {
		t.Errorf("before /default, voice should show as default:\n%s", txt)
	}
	m.command("/default")
	if txt := configText(m.o.cfg, func(s string) string { return "[" + s + "]" }); !strings.Contains(txt, "voice           [bm_george]") || !strings.Contains(txt, "sentence-pause  [0.4]") {
		t.Errorf("after /default, saved values should be highlighted:\n%s", txt)
	}
	cfg, _ := loadConfig()
	if cfg["voice"] != "bm_george" || cfg["speed"] != "1.25" || cfg["lang"] != "en-gb" || cfg["sentence-pause"] != "0.4" {
		t.Fatalf("/default not saved: %v", cfg)
	}

	// Completion: "/mo" offers /model first; "/voice BE" (any case) offers bella; plain text gets no menu.
	m.input.SetValue("/mo")
	if got := m.input.CurrentSuggestion(); got != "/model" || !strings.Contains(m.menu(), "/model kokoro-int8-multi-lang-v1_0") {
		t.Errorf("/mo: suggestion %q, menu:\n%s", got, m.menu())
	}
	m.input.SetValue("/voice BE")
	if got := m.input.CurrentSuggestion(); got != "/voice bella" {
		t.Errorf("/voice BE: suggestion %q", got)
	}
	m.input.SetValue("hello")
	if m.menu() != "" {
		t.Error("plain text must not show a menu")
	}
	for _, c := range tuiCommands {
		if !strings.Contains(helpText(), "/"+c.name) {
			t.Errorf("help is missing /%s", c.name)
		}
	}

	// /save needs audio; fake some and an engine rate without loading a model.
	m.last, m.eng = make([]float32, 2400), &engine{rate: 24000}
	out := filepath.Join(t.TempDir(), "a", "b.wav")
	m.command("/save " + out)
	if st, err := os.Stat(out); err != nil || st.Size() != 44+4800 {
		t.Fatalf("/save: %v", err)
	}
}
