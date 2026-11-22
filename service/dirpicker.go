package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// dirPicker is the /dir folder browser: Enter opens the highlighted folder, Esc closes,
// and the last entry creates a new folder.
type dirPicker struct {
	entries []string // "..", sub folders ending in "/", newFolderEntry
	sel     int
	naming  bool // typing a new folder name in the input box
	err     string
}

const (
	newFolderEntry   = "+ New folder…"
	inputPlaceholder = "type text to speak, or / for commands"
	namePlaceholder  = "new folder name"
)

// listDirs returns the picker entries for dir; hidden folders are left out.
func listDirs(dir string) ([]string, error) {
	var out []string
	if filepath.Dir(dir) != dir {
		out = append(out, "..")
	}
	ents, err := os.ReadDir(dir) // sorted by name
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		isDir := e.IsDir()
		if e.Type()&fs.ModeSymlink != 0 { // a link to a folder counts as a folder
			st, serr := os.Stat(filepath.Join(dir, e.Name()))
			isDir = serr == nil && st.IsDir()
		}
		if isDir {
			out = append(out, e.Name()+"/")
		}
	}
	return append(out, newFolderEntry), err
}

// resolveDir turns a /dir argument (absolute, relative to base, or ~/...) into an existing folder.
func resolveDir(base, arg string) (string, error) {
	if rest, ok := strings.CutPrefix(arg, "~"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		arg = home + rest
	}
	if !filepath.IsAbs(arg) {
		arg = filepath.Join(base, arg)
	}
	arg = filepath.Clean(arg)
	st, err := os.Stat(arg)
	if err != nil {
		return "", fmt.Errorf("no such folder: %s", tildePath(arg))
	}
	if !st.IsDir() {
		return "", fmt.Errorf("not a folder: %s", tildePath(arg))
	}
	return arg, nil
}

func validFolderName(name string) error {
	switch {
	case name == "":
		return errors.New("type a name first")
	case name == "." || name == "..":
		return errors.New("that name is reserved")
	case strings.ContainsAny(name, `/\`):
		return errors.New("use a single folder name, not a path")
	}
	return nil
}

// tildePath shortens a path inside the home folder to ~/...
func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && (p == home || strings.HasPrefix(p, home+string(os.PathSeparator))) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func (m *tui) setDir(dir string) {
	m.dir, m.dirChanged = dir, true
}

func (m *tui) openPicker() {
	m.picker = &dirPicker{}
	m.refreshPicker()
}

func (m *tui) refreshPicker() {
	entries, err := listDirs(m.dir)
	m.picker.entries, m.picker.sel, m.picker.err = entries, 0, ""
	if err != nil {
		m.picker.err = "can't read this folder: " + err.Error()
	}
}

// pickerKey handles every key while the picker is open.
func (m *tui) pickerKey(msg tea.KeyPressMsg) tea.Cmd {
	p := m.picker
	if p.naming {
		switch msg.String() {
		case "esc":
			p.naming, p.err = false, ""
			m.input.Placeholder = inputPlaceholder
			m.setInput("")
		case "enter":
			name := strings.TrimSpace(m.input.Value())
			if err := validFolderName(name); err != nil {
				p.err = err.Error()
				return nil
			}
			path := filepath.Join(m.dir, name)
			if err := os.Mkdir(path, 0o755); err != nil {
				if errors.Is(err, fs.ErrExist) {
					p.err = name + " already exists"
				} else {
					p.err = err.Error()
				}
				return nil
			}
			m.setDir(path)
			p.naming = false
			m.input.Placeholder = inputPlaceholder
			m.setInput("")
			m.refreshPicker()
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return cmd
		}
		return nil
	}
	switch msg.String() {
	case "up", "down":
		step := map[string]int{"up": -1, "down": 1}[msg.String()]
		p.sel = (p.sel + step + len(p.entries)) % len(p.entries)
	case "esc":
		m.picker = nil
		return done(dim.Render("dir → ") + cyan.Render(tildePath(m.dir)))
	case "enter":
		switch e := p.entries[p.sel]; e {
		case newFolderEntry:
			p.naming, p.err = true, ""
			m.input.Placeholder = namePlaceholder
			m.setInput("")
		case "..":
			m.setDir(filepath.Dir(m.dir))
			m.refreshPicker()
		default:
			m.setDir(filepath.Join(m.dir, strings.TrimSuffix(e, "/")))
			m.refreshPicker()
		}
	}
	return nil
}

func (m *tui) pickerView() string {
	p := m.picker
	const rows = 10
	var b strings.Builder
	b.WriteString("  " + accent.Render("folder ") + cyan.Render(tildePath(m.dir)) + "\n")
	start := min(max(0, p.sel-rows+1), max(0, len(p.entries)-rows))
	for i := start; i < min(len(p.entries), start+rows); i++ {
		e := p.entries[i]
		style := dim
		if e == newFolderEntry {
			style = green
		}
		if i == p.sel && !p.naming {
			b.WriteString(accent.Render("  › "+e) + "\n")
		} else {
			b.WriteString(style.Render("    "+e) + "\n")
		}
	}
	if more := len(p.entries) - rows; more > 0 {
		b.WriteString(dim.Render(fmt.Sprintf("    … %d more, ↑/↓ to scroll", more)) + "\n")
	}
	if p.err != "" {
		b.WriteString(red.Render("  ✗ "+p.err) + "\n")
	}
	hint := "↑/↓ choose · Enter open · Esc done"
	if p.naming {
		hint = "Enter create · Esc back"
	}
	b.WriteString(dim.Render("  " + hint))
	return b.String()
}
