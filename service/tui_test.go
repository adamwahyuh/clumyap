package main

import (
	"os"
	"path/filepath"
	"reflect"
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
	for _, c := range []string{"/save", "/replay", "/bogus", "/help", "/voice", "/model", "/speed", "/config", "/info"} {
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
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
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
	tm := newTUI(o)
	b := banner(tm.o, tm.dir)
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

func TestDirPicker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "work")
	for _, d := range []string{"b-folder", "a-folder", ".hidden"} {
		os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	os.WriteFile(filepath.Join(root, "file.txt"), nil, 0o644)
	os.Symlink(filepath.Join(root, "a-folder"), filepath.Join(root, "link"))

	got, err := listDirs(root)
	want := []string{"..", "a-folder/", "b-folder/", "link/", newFolderEntry}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("listDirs = %q, %v; want %q", got, err, want)
	}
	if got, _ := listDirs("/"); got[0] == ".." {
		t.Error("the root folder has no ..")
	}

	for arg, want := range map[string]string{"a-folder": root + "/a-folder", "..": home, "~/work/b-folder": root + "/b-folder", root: root} {
		if got, err := resolveDir(root, arg); err != nil || got != want {
			t.Errorf("resolveDir(%q) = %q, %v; want %q", arg, got, err, want)
		}
	}
	for _, bad := range []string{"nope", "file.txt"} {
		if _, err := resolveDir(root, bad); err == nil {
			t.Errorf("resolveDir(%q) should fail", bad)
		}
	}

	o, _ := parseArgs(nil, map[string]string{})
	m := newTUI(o)
	m.dir = root
	key := func(s string) { m.Update(keyPress(s)) }
	m.command("/dir")
	if m.picker == nil {
		t.Fatal("/dir should open the picker")
	}
	key("down") // a-folder/
	key("enter")
	if m.dir != root+"/a-folder" || m.picker.entries[0] != ".." {
		t.Fatalf("enter should open a-folder: %q %q", m.dir, m.picker.entries)
	}
	key("enter") // ".." is first
	if m.dir != root {
		t.Fatalf(".. should go back up: %q", m.dir)
	}

	// New folder: last entry, type a name, Enter creates it and moves in.
	key("up") // wraps to "+ New folder…"
	key("enter")
	if !m.picker.naming {
		t.Fatal("the new-folder entry should ask for a name")
	}
	m.setInput("bad/name")
	key("enter")
	if m.picker.err == "" || m.dir != root {
		t.Fatalf("a path as a name must be refused: err %q dir %q", m.picker.err, m.dir)
	}
	m.setInput("b-folder")
	key("enter")
	if !strings.Contains(m.picker.err, "already exists") {
		t.Fatalf("an existing name must be refused: %q", m.picker.err)
	}
	m.setInput("recordings")
	key("enter")
	if st, err := os.Stat(root + "/recordings"); err != nil || !st.IsDir() || m.dir != root+"/recordings" || m.picker.naming {
		t.Fatalf("new folder not created and opened: %v dir %q", err, m.dir)
	}
	key("esc")
	if m.picker != nil {
		t.Fatal("esc should close the picker")
	}

	// /save writes to the chosen folder; /default stores it because /dir was used.
	m.last, m.eng = make([]float32, 240), &engine{rate: 24000}
	m.command("/save")
	if files, _ := filepath.Glob(root + "/recordings/clumyap-*.mp3"); len(files) != 1 {
		t.Fatalf("/save should write into the chosen folder, found %q", files)
	}
	m.command("/default")
	if cfg, _ := loadConfig(); cfg["output-dir"] != root+"/recordings" {
		t.Fatalf("/default should store the /dir folder, got %q", cfg["output-dir"])
	}
	// Without /dir, /default leaves output-dir alone.
	m2 := newTUI(o)
	m2.o.cfg = map[string]string{}
	m2.command("/default")
	if _, ok := m2.o.cfg["output-dir"]; ok {
		t.Fatal("/default must not store the start folder when /dir was not used")
	}
}

func TestInfo(t *testing.T) {
	txt := infoText()
	for _, want := range []string{"v" + version(), appAuthor, appEmail, "Kokoro-82M", "hexgrad", "Apache-2.0", "espeak-ng", "GPL-3.0", "shine-mp3", "LGPL-2.0", "GPL-3.0-or-later"} {
		if !strings.Contains(txt, want) {
			t.Errorf("/info is missing %q", want)
		}
	}
	for _, line := range strings.Split(regexp.MustCompile(`\x1b\[[0-9;:]*m`).ReplaceAllString(txt, ""), "\n") {
		if w := lipgloss.Width(line); w > 80 {
			t.Errorf("/info line is %d wide, wraps in an 80-column terminal: %q", w, line)
		}
	}
}
