package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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

	// Completion: "/mo" + Tab gives /model; down then Tab picks the next match; any case works.
	key := func(s string) { m.Update(keyPress(s)) }
	m.setInput("/mo")
	if !strings.Contains(m.menu(), "/model kokoro-int8-multi-lang-v1_0") {
		t.Errorf("/mo menu:\n%s", m.menu())
	}
	key("tab")
	if m.input.Value() != "/model" {
		t.Errorf("/mo + tab = %q", m.input.Value())
	}
	m.setInput("/model ")
	key("down")
	key("tab")
	if m.input.Value() != "/model kokoro-int8-multi-lang-v1_0" {
		t.Errorf("/model + down + tab = %q", m.input.Value())
	}
	m.setInput("/voice BE")
	key("tab")
	if m.input.Value() != "/voice bella" {
		t.Errorf("/voice BE + tab = %q", m.input.Value())
	}
	m.setInput("hello")
	if m.menu() != "" {
		t.Error("plain text must not show a menu")
	}

	// New lines: a trailing backslash + enter, and ctrl+j, insert a line break instead of sending.
	m.setInput(`First line\`)
	key("enter")
	key("ctrl+j")
	if m.input.Value() != "First line\n\n" {
		t.Errorf("newline keys: %q", m.input.Value())
	}
	// A multi-line "/..." is text, not a command; the menu stays closed.
	m.setInput("/voice\nbella")
	if m.matches() != nil {
		t.Error("multi-line input must not complete")
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

// keyPress builds the key event Bubble Tea sends for names like "tab", "enter", "down", "ctrl+j".
func keyPress(name string) tea.KeyPressMsg {
	switch name {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "ctrl+j":
		return tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}
	}
	panic("unknown key " + name)
}
