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

const tuiHelp = `Type text and press Enter to hear it.
  /voice [name]       switch voice (no name: list voices)
  /speed N            speed 0.5-2.0
  /lang en-us|en-gb   accent of the pronunciation
  /model [name]       switch model (no name: list models)
  /replay             play the last audio again
  /stop               stop playback (or Esc)
  /save [file]        save the last audio (.mp3/.wav, default: output-dir)
  /default            keep the current voice, speed, lang and model as defaults
  /help               this help
  /exit               quit (or Ctrl+C)`

var (
	accent = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	dim    = lipgloss.NewStyle().Faint(true)
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	box    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(0, 1)
)

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
	in.Placeholder = "type text to speak, or /help"
	in.Focus()
	return &tui{o: o, input: in, spin: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(accent))}
}

func say(style lipgloss.Style, s string) tea.Cmd { return tea.Println(style.Render(s)) }
func fail(err error) tea.Cmd                     { return say(red, "✗ "+err.Error()) }

func (m *tui) Init() tea.Cmd {
	banner := tea.Println(accent.Bold(true).Render("clumyap") + dim.Render(" · offline text-to-speech · /help for commands"))
	return tea.Batch(banner, m.load(m.o.model))
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
		return m, say(dim, "✓ model "+m.o.model+" ready")
	case synthMsg:
		m.busy = ""
		if msg.err != nil {
			return m, fail(msg.err)
		}
		m.last = msg.samples
		secs := float64(len(msg.samples)) / float64(m.eng.rate)
		return m, tea.Batch(say(dim, fmt.Sprintf("✓ %.1fs audio · made in %.1fs", secs, msg.took.Seconds())), m.play())
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
	echo := tea.Println(accent.Render("> ") + line)
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
		return say(dim, tuiHelp)
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
		return say(dim, name+" → "+val)
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
		return say(green, "✓ saved "+path)
	case "default":
		sets := []string{"voice=" + m.o.voice, "speed=" + strconv.FormatFloat(m.o.speed, 'f', -1, 64), "lang=" + m.o.lang, "model=" + m.o.model}
		if err := updateConfig(m.o.cfg, sets, nil); err != nil {
			return fail(err)
		}
		return say(green, "✓ saved as defaults")
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
		b.WriteString(m.spin.View() + " " + dim.Render(m.busy+m.pct) + "\n")
	}
	b.WriteString(box.Render(m.input.View()) + "\n")
	mod, _ := findModel(m.o.model)
	status := fmt.Sprintf(" %s · speed %s · %s · %s model", m.o.voice, strconv.FormatFloat(m.o.speed, 'f', -1, 64), m.o.lang, mod.short)
	if m.player != nil {
		status += " · ▶ esc to stop"
	}
	b.WriteString(dim.Render(status))
	return tea.NewView(b.String())
}
