package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// tuiCommands is the single source for /help, Tab completion and the suggestion menu.
var tuiCommands = []struct{ name, args, desc string }{
	{"voice", "[name]", "switch voice (no name: list voices)"},
	{"speed", "N", "speed 0.5-2.0"},
	{"lang", "en-us|en-gb", "accent of the pronunciation"},
	{"model", "[name]", "switch model (no name: list models)"},
	{"replay", "", "play the last audio again"},
	{"stop", "", "stop playback (or Esc)"},
	{"save", "[file]", "save the last audio (.mp3/.wav, default: output-dir)"},
	{"default", "", "keep the current voice, speed, lang and model as defaults"},
	{"help", "", "show commands"},
	{"exit", "", "quit (or Ctrl+C)"},
}

// Mid-tone colors so text stays readable on both dark and light terminals.
var (
	cPurple = lipgloss.Color("#8B5CF6")
	cPink   = lipgloss.Color("#EC4899")
	cAmber  = lipgloss.Color("#D97706")
	cCyan   = lipgloss.Color("#06B6D4")
	cGreen  = lipgloss.Color("#16A34A")
	cRed    = lipgloss.Color("#EF4444")
	cGray   = lipgloss.Color("#6B7280")

	accent = lipgloss.NewStyle().Foreground(cPurple).Bold(true)
	dim    = lipgloss.NewStyle().Foreground(cGray)
	red    = lipgloss.NewStyle().Foreground(cRed)
	green  = lipgloss.NewStyle().Foreground(cGreen)
	pink   = lipgloss.NewStyle().Foreground(cPink)
	amber  = lipgloss.NewStyle().Foreground(cAmber)
	cyan   = lipgloss.NewStyle().Foreground(cCyan)
	box    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

func banner() string {
	word := "clumyap"
	colors := lipgloss.Blend1D(len(word), cPurple, cPink, cAmber)
	var b strings.Builder
	for i, r := range word {
		b.WriteString(lipgloss.NewStyle().Foreground(colors[i]).Bold(true).Render(string(r)))
	}
	return "♪ " + b.String() + dim.Render("  offline text-to-speech · ") + cyan.Render("/help") + dim.Render(" for commands · ") + cyan.Render("Tab") + dim.Render(" to complete")
}

func helpText() string {
	var b strings.Builder
	b.WriteString(dim.Render("Type text and press Enter to hear it.") + "\n")
	for _, c := range tuiCommands {
		left := "/" + c.name
		if c.args != "" {
			left += " " + c.args
		}
		b.WriteString("  " + cyan.Render(fmt.Sprintf("%-20s", left)) + dim.Render(c.desc) + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// suggestions are full input lines the textinput offers for prefix completion; desc feeds the menu.
func suggestions() (lines []string, desc map[string]string) {
	desc = map[string]string{}
	add := func(line, d string) { lines = append(lines, line); desc[line] = d }
	for _, c := range tuiCommands {
		add("/"+c.name, c.desc)
	}
	groups := map[string]string{"af": "American female", "am": "American male", "bf": "British female", "bm": "British male"}
	for _, v := range voices {
		add("/voice "+v[3:], groups[v[:2]])
	}
	for _, m := range models {
		add("/model "+m.name, m.size+" · "+m.note)
	}
	add("/lang en-us", "American pronunciation")
	add("/lang en-gb", "British pronunciation")
	return lines, desc
}

type (
	loadedMsg struct {
		eng *engine
		err error
	}
	synthMsg struct {
		samples []float32
		took    time.Duration
		err     error
	}
	progressMsg struct {
		para, total int
		pct         float32
	}
	playDoneMsg struct{ cmd *exec.Cmd }
)

// tui is the `clumyap start` session. Settings changed here last for the session; /default saves them.
type tui struct {
	o      options
	eng    *engine
	prog   *tea.Program
	input  textinput.Model
	spin   spinner.Model
	busy   string // what the engine is doing; "" = idle. The engine runs one job at a time.
	pct    string
	last   []float32
	player *exec.Cmd
	desc   map[string]string
}

func runTUI(o options) error {
	if st, _ := os.Stdin.Stat(); st == nil || st.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("%w: start needs an interactive terminal", errUsage)
	}
	m := newTUI(o)
	m.prog = tea.NewProgram(m)
	_, err := m.prog.Run()
	m.stop()
	// The engine is not closed: a generation may still be running, and the process exits right after.
	return err
}

func newTUI(o options) *tui {
	o.voice = voices[voiceID(o.voice)]
	in := textinput.New()
	in.Prompt = "› "
	in.Placeholder = "type text to speak, or / for commands"
	st := in.Styles()
	st.Focused.Prompt = accent
	st.Focused.Placeholder = dim
	st.Focused.Suggestion = dim
	in.SetStyles(st)
	lines, desc := suggestions()
	in.ShowSuggestions = true
	in.SetSuggestions(lines)
	in.Focus()
	return &tui{o: o, input: in, desc: desc, spin: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(pink))}
}

func say(style lipgloss.Style, s string) tea.Cmd { return tea.Println(style.Render(s)) }
func fail(err error) tea.Cmd                     { return say(red, "✗ "+err.Error()) }
func done(s string) tea.Cmd                      { return tea.Println(green.Render("✓ ") + s) }

func (m *tui) Init() tea.Cmd {
	return tea.Batch(tea.Println(banner()), m.load(m.o.model))
}

func (m *tui) load(name string) tea.Cmd {
	mod, _ := findModel(name)
	m.busy = "loading " + name
	if !modelInstalled(mod) {
		m.busy = "downloading " + name + " (" + mod.size + ")"
	}
	return tea.Batch(m.spin.Tick, func() tea.Msg {
		if _, err := ensureModel(mod, true); err != nil {
			return loadedMsg{err: err}
		}
		e, err := loadEngine(mod)
		return loadedMsg{eng: e, err: err}
	})
}

func (m *tui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.input.SetWidth(max(10, msg.Width-6))
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+d":
			if m.input.Value() == "" {
				return m, tea.Quit
			}
		case "esc":
			m.stop()
			return m, nil
		case "enter":
			return m, m.submit()
		}
	case spinner.TickMsg:
		if m.busy == "" {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case progressMsg:
		m.pct = fmt.Sprintf(" %.0f%%", msg.pct*100)
		if msg.total > 1 {
			m.pct = fmt.Sprintf(" [%d/%d]%s", msg.para, msg.total, m.pct)
		}
		return m, nil
	case loadedMsg:
		m.busy = ""
		if msg.err != nil {
			return m, fail(msg.err)
		}
		if m.eng != nil {
			m.eng.close()
		}
		m.eng, m.o.model = msg.eng, msg.eng.model.name
		return m, done(dim.Render("model ") + green.Render(m.o.model) + dim.Render(" ready"))
	case synthMsg:
		m.busy = ""
		if msg.err != nil {
			return m, fail(msg.err)
		}
		m.last = msg.samples
		secs := float64(len(msg.samples)) / float64(m.eng.rate)
		return m, tea.Batch(done(amber.Render(fmt.Sprintf("%.1fs", secs))+dim.Render(" audio · made in ")+amber.Render(fmt.Sprintf("%.1fs", msg.took.Seconds()))), m.play())
	case playDoneMsg:
		if m.player == msg.cmd {
			m.player = nil
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *tui) submit() tea.Cmd {
	line := strings.TrimSpace(m.input.Value())
	if line == "" {
		return nil
	}
	m.input.Reset()
	echo := tea.Println(accent.Render("› ") + line)
	if strings.HasPrefix(line, "/") {
		echo = tea.Println(accent.Render("› ") + cyan.Render(line))
	}
	if strings.HasPrefix(line, "/") {
		return tea.Sequence(echo, m.command(line))
	}
	switch {
	case m.busy != "":
		return tea.Sequence(echo, fail(errors.New("busy ("+m.busy+"), try again in a moment")))
	case m.eng == nil:
		return tea.Sequence(echo, fail(errors.New("no model loaded, see /model")))
	}
	m.stop()
	m.busy, m.pct = "generating", ""
	e, voice, lang, speed, p := m.eng, m.o.voice, m.o.lang, m.o.speed, m.prog
	return tea.Batch(echo, m.spin.Tick, func() tea.Msg {
		start := time.Now()
		s, err := e.synth(line, voice, lang, speed, func(i, n int, pct float32) {
			if p != nil {
				p.Send(progressMsg{i, n, pct})
			}
		})
		return synthMsg{s, time.Since(start), err}
	})
}

func (m *tui) command(line string) tea.Cmd {
	name, arg, _ := strings.Cut(strings.TrimPrefix(line, "/"), " ")
	arg = strings.TrimSpace(arg)
	switch name {
	case "help":
		return tea.Println(helpText())
	case "exit", "quit":
		return tea.Quit
	case "stop":
		m.stop()
		return say(dim, "stopped")
	case "voice", "speed", "lang", "model":
		if arg == "" {
			switch name {
			case "voice":
				return say(dim, voicesText())
			case "model":
				return say(dim, modelsText(m.o.model))
			}
			return fail(fmt.Errorf("usage: /%s VALUE", name))
		}
		val, err := normalizeSetting(name, arg)
		if err != nil {
			return fail(err)
		}
		switch name {
		case "voice":
			m.o.voice = val
		case "speed":
			m.o.speed, _ = strconv.ParseFloat(val, 64)
		case "lang":
			m.o.lang = val
		case "model":
			if m.busy != "" {
				return fail(errors.New("busy (" + m.busy + "), try again in a moment"))
			}
			if val == m.o.model && m.eng != nil {
				return say(dim, "already using "+val)
			}
			return m.load(val)
		}
		return done(dim.Render(name+" → ") + pink.Render(val))
	case "replay":
		if m.last == nil {
			return fail(errors.New("nothing to replay yet"))
		}
		return m.play()
	case "save":
		if m.last == nil {
			return fail(errors.New("nothing to save yet"))
		}
		path := arg
		if path == "" {
			path = filepath.Join(m.o.cfg["output-dir"], "clumyap-"+time.Now().Format("20060102-150405")+".mp3")
		}
		path, err := expandPath(path)
		if err != nil {
			return fail(err)
		}
		if ext := strings.ToLower(filepath.Ext(path)); ext != ".mp3" && ext != ".wav" {
			return fail(errors.New("file must end in .mp3 or .wav"))
		}
		if err := writeFile(path, m.last, m.eng.rate); err != nil {
			return fail(err)
		}
		return done(dim.Render("saved ") + cyan.Render(path))
	case "default":
		sets := []string{"voice=" + m.o.voice, "speed=" + strconv.FormatFloat(m.o.speed, 'f', -1, 64), "lang=" + m.o.lang, "model=" + m.o.model}
		if err := updateConfig(m.o.cfg, sets, nil); err != nil {
			return fail(err)
		}
		return done(dim.Render("saved as defaults"))
	}
	return fail(fmt.Errorf("unknown command /%s, see /help", name))
}

// play writes the last audio to a temp WAV and plays it with afplay; any earlier playback stops.
func (m *tui) play() tea.Cmd {
	m.stop()
	f, err := os.CreateTemp("", "clumyap-*.wav")
	if err != nil {
		return fail(err)
	}
	err = writeWAV(f, m.last, m.eng.rate)
	f.Close()
	c := exec.Command("afplay", f.Name())
	if err == nil {
		err = c.Start()
	}
	if err != nil {
		os.Remove(f.Name())
		return fail(err)
	}
	m.player = c
	return func() tea.Msg {
		c.Wait()
		os.Remove(f.Name())
		return playDoneMsg{c}
	}
}

func (m *tui) stop() {
	if m.player != nil {
		m.player.Process.Kill()
		m.player = nil
	}
}

func (m *tui) View() tea.View {
	var b strings.Builder
	if m.busy != "" {
		b.WriteString(m.spin.View() + " " + pink.Render(m.busy) + amber.Render(m.pct) + "\n")
	}
	border := cPurple
	if m.busy != "" {
		border = cAmber
	}
	b.WriteString(box.BorderForeground(border).Render(m.input.View()) + "\n")
	if menu := m.menu(); menu != "" {
		b.WriteString(menu + "\n")
	}
	mod, _ := findModel(m.o.model)
	sep := dim.Render(" · ")
	b.WriteString(" " + pink.Render("♪ "+m.o.voice) + sep + amber.Render(strconv.FormatFloat(m.o.speed, 'f', -1, 64)+"x") +
		sep + cyan.Render(m.o.lang) + sep + green.Render(mod.short+" model"))
	if m.player != nil {
		b.WriteString(sep + pink.Render("▶ playing") + dim.Render(" esc to stop"))
	}
	return tea.NewView(b.String())
}

// menu lists the completions for a "/..." input, highlighting the one Tab will accept.
func (m *tui) menu() string {
	val := m.input.Value()
	matches := m.input.MatchedSuggestions()
	if !strings.HasPrefix(val, "/") || len(matches) == 0 || (len(matches) == 1 && matches[0] == val) {
		return ""
	}
	const rows = 6
	cur := m.input.CurrentSuggestionIndex()
	start := min(max(0, cur-rows+1), max(0, len(matches)-rows))
	end := min(len(matches), start+rows)
	width := 0
	for _, s := range matches[start:end] {
		width = max(width, len(s)+2)
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		line := fmt.Sprintf("%-*s", width, matches[i])
		if i == cur {
			b.WriteString(accent.Render("  › "+line) + dim.Render(m.desc[matches[i]]) + "\n")
		} else {
			b.WriteString(dim.Render("    "+line+m.desc[matches[i]]) + "\n")
		}
	}
	if more := len(matches) - rows; more > 0 {
		b.WriteString(dim.Render(fmt.Sprintf("    … %d more, ↑/↓ to browse", more)) + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}
