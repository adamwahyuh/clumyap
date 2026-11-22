package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

func TestBannerAndLogo(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(version()) {
		t.Fatalf("VERSION %q is not a version number", version())
	}
	lines := logoLines()
	if len(lines) != len(logoArt)/2 {
		t.Fatalf("want %d logo lines, got %d", len(logoArt)/2, len(lines))
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w != 16 {
			t.Errorf("logo line %d is %d cells wide, want 16", i, w)
		}
	}
	o, _ := parseArgs(nil, map[string]string{})
	b := banner(newTUI(o).o)
	for _, want := range []string{"v" + version(), "kokoro-multi-lang-v1_0", "af_heart", "/help"} {
		if !strings.Contains(b, want) {
			t.Errorf("banner is missing %q", want)
		}
	}
}

func TestTUIHistory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	o, _ := parseArgs(nil, map[string]string{})
	m := newTUI(o)
	key := func(s string) { m.Update(keyPress(s)) }
	send := func(s string) { m.setInput(s); m.submit() }

	key("up") // no history yet: nothing happens
	if m.input.Value() != "" {
		t.Fatalf("up with no history changed input to %q", m.input.Value())
	}
	send("/voice bella")
	send("/model")
	send("/model") // same as the previous entry: stored once
	send("First line\nsecond line")
	if len(m.history) != 3 {
		t.Fatalf("history = %q", m.history)
	}

	m.setInput("my draft")
	key("up")
	if m.input.Value() != "First line\nsecond line" {
		t.Fatalf("up 1 = %q", m.input.Value())
	}
	key("up") // cursor is on the last line of a two-line entry: moves to its first line
	if m.input.Value() != "First line\nsecond line" || m.input.Line() != 0 {
		t.Fatalf("up inside a multi-line entry should move lines, got %q line %d", m.input.Value(), m.input.Line())
	}
	key("up")
	if m.input.Value() != "/model" {
		t.Fatalf("up 3 = %q", m.input.Value())
	}
	key("up") // "/model" would open the completion menu; while browsing, up keeps going back
	if m.input.Value() != "/voice bella" {
		t.Fatalf("up 4 = %q", m.input.Value())
	}
	key("up") // oldest: stays
	if m.input.Value() != "/voice bella" {
		t.Fatalf("up past oldest = %q", m.input.Value())
	}
	key("down")
	key("down")
	if m.input.Value() != "First line\nsecond line" {
		t.Fatalf("down 2 = %q", m.input.Value())
	}
	key("down")
	if m.input.Value() != "my draft" {
		t.Fatalf("down past newest should restore the draft, got %q", m.input.Value())
	}

	// Editing a recalled entry ends browsing, so the menu works again.
	key("up")
	key("up")
	key("up") // "/model"
	m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	if m.input.Value() != "/model " || len(m.matches()) == 0 {
		t.Fatalf("after editing, completion should be back: %q, %d matches", m.input.Value(), len(m.matches()))
	}
}
