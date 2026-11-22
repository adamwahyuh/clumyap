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

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// tuiCommands is the single source for /help, Tab completion and the suggestion menu.
var tuiCommands = []struct{ name, args, desc string }{
	{"voice", "[name]", "switch voice (no name: list voices)"},
	{"speed", "N", "speed 0.5-2.0"},
	{"pause", "N", "seconds of silence between paragraphs, 0-3"},
	{"sentence-pause", "N", "extra silence after each sentence, 0-3 (0 = natural flow)"},
	{"lang", "en-us|en-gb", "accent of the pronunciation"},
	{"model", "[name]", "switch model (no name: list models)"},
	{"pronounce", "[word = say as]", "teach how to say a word (no args: list, -word: remove)"},
	{"dir", "[path]", "choose the folder /save writes to (no path: browse, .. goes up)"},
	{"replay", "", "play the last audio again"},
	{"stop", "", "stop playback (or Esc)"},
	{"save", "[file]", "save the last audio (.mp3/.wav, default: output-dir)"},
	{"config", "", "show saved settings (colored = saved, rest = built-in default)"},
	{"default", "", "keep the current voice, speed, pauses, lang and model as defaults"},
	{"update", "", "update clumyap to the latest release"},
	{"uninstall", "[yes|all]", "remove clumyap (all: also models and settings)"},
	{"info", "", "about clumyap: version, maker and credits"},
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

// banner is the start-up card: logo, version, model, voice and the folder /save writes to.
func banner(o options, dir string) string {
	word := "clumyap"
	colors := lipgloss.Blend1D(len(word), cPurple, cPink, cAmber)
	var name strings.Builder
	for i, r := range word {
		name.WriteString(lipgloss.NewStyle().Foreground(colors[i]).Bold(true).Render(string(r)))
	}
	info := strings.Join([]string{
		name.String() + " " + dim.Render("v"+version()),
		dim.Render("offline text-to-speech"),
		"",
		dim.Render("model  ") + green.Render(o.model),
		dim.Render("voice  ") + pink.Render(o.voice),
		dim.Render("dir    ") + cyan.Render(tildePath(dir)),
	}, "\n")
	body := lipgloss.JoinHorizontal(lipgloss.Center, strings.Join(logoLines(), "\n"), "   ", info)
	card := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cPurple).Padding(0, 2).Render(body)
	hints := " " + cyan.Render("/help") + dim.Render(" commands · ") + cyan.Render("Tab") + dim.Render(" complete · ") +
		cyan.Render("shift/option+enter") + dim.Render(" new line")
	return card + "\n" + hints
}

func helpText() string {
	var b strings.Builder
	b.WriteString(dim.Render("Type text and press Enter to hear it. New line: Shift+Enter, Option+Enter, Ctrl+J, or end the line with \\.") + "\n")
	b.WriteString(dim.Render("A blank line between paragraphs adds the /pause silence.") + "\n")
	for _, c := range tuiCommands {
		left := "/" + c.name
		if c.args != "" {
			left += " " + c.args
		}
		b.WriteString("  " + cyan.Render(fmt.Sprintf("%-20s", left)) + dim.Render(c.desc) + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// suggestions are full input lines offered for Tab completion; desc feeds the menu.
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
	downloadMsg struct{ read, total int64 }
	updateMsg   struct {
		latest string // "" when already current
		err    error
	}
)

// tui is the `clumyap start` session. Settings changed here last for the session; /default saves them.
type tui struct {
	o      options
	eng    *engine
	prog   *tea.Program
	input  textarea.Model
	spin   spinner.Model
	busy   string // what the engine is doing; "" = idle. The engine runs one job at a time.
	pct    string
	last   []float32
	player *exec.Cmd
	played string   // the temp WAV being played; removed when playback stops
	sugs   []string // completion lines, see suggestions()
	desc   map[string]string
	sel    int // highlighted entry in the completion menu

	history []string // what was sent this session, oldest first
	histIdx int      // entry shown while browsing; len(history) = not browsing
	draft   string   // the unsent input, restored when browsing back past the newest entry

	dir        string     // where /save writes this session: output-dir, else the folder clumyap started in
	dirChanged bool       // set by /dir; only then does /default store dir as output-dir
	picker     *dirPicker // the /dir browser while open
}

func runTUI(o options) error {
	if st, _ := os.Stdin.Stat(); st == nil || st.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("%w: start needs an interactive terminal", errUsage)
	}
	// Start on a clean screen like Claude Code: cursor home + erase the visible screen, before Bubble Tea
	// starts (sent later through tea.Raw it arrives only at exit). The scrollback is kept (no \x1b[3J).
	enableVT()
	fmt.Print("\x1b[H\x1b[2J")
	m := newTUI(o)
	m.prog = tea.NewProgram(m)
	_, err := m.prog.Run()
	m.stop()
	// The engine is not closed: a generation may still be running, and the process exits right after.
	return err
}

func newTUI(o options) *tui {
	o.voice = voices[voiceID(o.voice)]
	in := textarea.New()
	in.Placeholder = inputPlaceholder
	in.ShowLineNumbers = false
	in.DynamicHeight, in.MinHeight, in.MaxHeight = true, 1, 10
	in.SetPromptFunc(2, func(p textarea.PromptInfo) string {
		if p.LineNumber == 0 {
			return "› "
		}
		return "  "
	})
	// Enter sends; these insert a line break. Plain terminals (e.g. macOS Terminal) send shift+enter
	// as enter, so option+enter, ctrl+j and a trailing "\" work everywhere.
	in.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", "alt+enter", "ctrl+j"))
	st := in.Styles()
	st.Focused.Prompt, st.Focused.Placeholder, st.Focused.CursorLine = accent, dim, lipgloss.NewStyle()
	in.SetStyles(st)
	in.Focus()
	lines, desc := suggestions()
	dir := o.cfg["output-dir"]
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return &tui{o: o, input: in, sugs: lines, desc: desc, dir: dir, spin: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(pink))}
}

// matches are the completions for a one-line "/..." input, case-insensitive by prefix.
func (m *tui) matches() []string {
	if m.histIdx < len(m.history) {
		return nil // browsing history: keep the arrows for history until the user edits
	}
	val := strings.ToLower(m.input.Value())
	if !strings.HasPrefix(val, "/") || strings.Contains(val, "\n") {
		return nil
	}
	var out []string
	for _, s := range m.sugs {
		if strings.HasPrefix(strings.ToLower(s), val) {
			out = append(out, s)
		}
	}
	if len(out) == 1 && strings.EqualFold(out[0], m.input.Value()) {
		return nil // already complete
	}
	return out
}

// showHistory puts history entry i in the input, or the saved draft when i is past the newest.
func (m *tui) showHistory(i int) {
	m.histIdx = i
	text := m.draft
	if i < len(m.history) {
		text = m.history[i]
	}
	m.input.SetValue(text)
	m.input.MoveToEnd()
	m.sel = 0
}

func (m *tui) setInput(s string) {
	m.input.SetValue(s)
	m.input.MoveToEnd()
	m.sel = 0
}

func say(style lipgloss.Style, s string) tea.Cmd { return tea.Println(style.Render(s)) }
func fail(err error) tea.Cmd                     { return say(red, "✗ "+err.Error()) }
func done(s string) tea.Cmd                      { return tea.Println(green.Render("✓ ") + s) }

func (m *tui) Init() tea.Cmd {
	return tea.Batch(tea.Println(banner(m.o, m.dir)), m.load(m.o.model))
}

func (m *tui) load(name string) tea.Cmd {
	mod, _ := findModel(name)
	m.busy, m.pct = "loading "+name, ""
	if !modelInstalled(mod) {
		m.busy = "downloading the voice model, once"
	}
	p := m.prog
	return tea.Batch(m.spin.Tick, func() tea.Msg {
		progress := func(read, total int64) {
			if p != nil {
				p.Send(downloadMsg{read, total})
			}
		}
		if _, err := ensureModel(mod, progress); err != nil {
			return loadedMsg{err: err}
		}
		e, err := loadEngine(mod)
		return loadedMsg{eng: e, err: err}
	})
}

func (m *tui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.input.SetWidth(max(10, msg.Width-4))
	case tea.KeyPressMsg:
		if m.picker != nil && msg.String() != "ctrl+c" {
			return m, m.pickerKey(msg)
		}
		matches := m.matches()
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
			if v := m.input.Value(); strings.HasSuffix(v, `\`) {
				m.setInput(strings.TrimSuffix(v, `\`) + "\n")
				return m, nil
			}
			return m, m.submit()
		case "tab":
			if len(matches) > 0 {
				m.setInput(matches[min(m.sel, len(matches)-1)])
			}
			return m, nil
		case "up", "down":
			if len(matches) > 0 { // with the menu open the arrows pick; otherwise they move between lines
				step := map[string]int{"up": -1, "down": 1}[msg.String()]
				m.sel = (m.sel + step + len(matches)) % len(matches)
				return m, nil
			}
			// On the first line up recalls older input, on the last line down newer; elsewhere they move lines.
			if msg.String() == "up" && m.input.Line() == 0 && m.histIdx > 0 {
				if m.histIdx == len(m.history) {
					m.draft = m.input.Value()
				}
				m.showHistory(m.histIdx - 1)
				return m, nil
			}
			if msg.String() == "down" && m.input.Line() == m.input.LineCount()-1 && m.histIdx < len(m.history) {
				m.showHistory(m.histIdx + 1)
				return m, nil
			}
		}
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			m.sel, m.histIdx = 0, len(m.history) // editing ends history browsing
		}
		return m, cmd
	case spinner.TickMsg:
		if m.busy == "" {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case downloadMsg:
		m.pct = "  " + progressBar(msg.read, msg.total)
		return m, nil
	case progressMsg:
		m.pct = fmt.Sprintf(" %.0f%%", msg.pct*100)
		if msg.total > 1 {
			m.pct = fmt.Sprintf(" [%d/%d]%s", msg.para, msg.total, m.pct)
		}
		return m, nil
	case loadedMsg:
		m.busy, m.pct = "", ""
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
	case updateMsg:
		m.busy = ""
		switch {
		case msg.err != nil:
			return m, fail(msg.err)
		case msg.latest == "":
			return m, done(dim.Render("clumyap ") + version() + dim.Render(" is the latest version"))
		}
		return m, done(dim.Render("updated to ") + green.Render(msg.latest) + dim.Render(". Restart clumyap (/exit) to use it."))
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
	if n := len(m.history); n == 0 || m.history[n-1] != line {
		m.history = append(m.history, line)
	}
	m.histIdx, m.draft = len(m.history), ""
	m.setInput("")
	isCommand := strings.HasPrefix(line, "/") && !strings.Contains(line, "\n")
	shown := strings.ReplaceAll(line, "\n", "\n  ")
	if isCommand {
		shown = cyan.Render(line)
	}
	echo := tea.Println(accent.Render("› ") + shown)
	if isCommand {
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
	e, o, p := m.eng, m.o, m.prog
	return tea.Batch(echo, m.spin.Tick, func() tea.Msg {
		start := time.Now()
		s, err := e.synth(line, o, func(i, n int, pct float32) {
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
	case "info":
		return tea.Println(infoText())
	case "uninstall":
		switch arg {
		case "":
			if !installedByScript() {
				return fail(errUninstallSource)
			}
			return say(dim, uninstallSummary()+"\n  /uninstall yes   remove clumyap, keep "+dataSummary()+
				"\n  /uninstall all   remove clumyap and "+dataSummary())
		case "yes", "all":
			m.stop()
			var lines []string
			err := uninstall(arg == "all", tuiSteps{&lines})
			report := tea.Println(strings.Join(lines, "\n"))
			if err != nil {
				return tea.Sequence(report, fail(err))
			}
			return tea.Sequence(report, tea.Println(dim.Render("clumyap is uninstalled. To reinstall: "+reinstallHint())), tea.Quit)
		}
		return fail(errors.New("use /uninstall, /uninstall yes or /uninstall all"))
	case "update":
		if m.busy != "" {
			return fail(errors.New("busy (" + m.busy + "), try again in a moment"))
		}
		m.busy = "checking for updates"
		return tea.Batch(m.spin.Tick, func() tea.Msg {
			latest, err := checkUpdate()
			if err != nil || latest == "" {
				return updateMsg{latest, err}
			}
			// The script's output would scribble over the TUI, so it is captured and shown only on failure.
			if out, err := installerCommand().CombinedOutput(); err != nil {
				return updateMsg{err: fmt.Errorf("update failed: %v\n%s", err, strings.TrimSpace(string(out)))}
			}
			return updateMsg{latest: latest}
		})
	case "exit", "quit":
		return tea.Quit
	case "stop":
		m.stop()
		return say(dim, "stopped")
	case "voice", "speed", "pause", "sentence-pause", "lang", "model":
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
		case "pause":
			m.o.pause, _ = strconv.ParseFloat(val, 64)
		case "sentence-pause":
			m.o.sentencePause, _ = strconv.ParseFloat(val, 64)
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
	case "config":
		return tea.Println(configText(m.o.cfg, func(s string) string { return pink.Render(s) }))
	case "replay":
		if m.last == nil {
			return fail(errors.New("nothing to replay yet"))
		}
		return m.play()
	case "pronounce":
		return m.pronounce(arg)
	case "dir":
		if arg == "" {
			m.openPicker()
			return nil
		}
		dir, err := resolveDir(m.dir, arg)
		if err != nil {
			return fail(err)
		}
		m.setDir(dir)
		return done(dim.Render("dir → ") + cyan.Render(tildePath(dir)))
	case "save":
		if m.last == nil {
			return fail(errors.New("nothing to save yet"))
		}
		path := arg
		if path == "" {
			path = filepath.Join(m.dir, "clumyap-"+time.Now().Format("20060102-150405")+".mp3")
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
		sets := []string{"voice=" + m.o.voice, "speed=" + strconv.FormatFloat(m.o.speed, 'f', -1, 64), "pause=" + strconv.FormatFloat(m.o.pause, 'f', -1, 64), "sentence-pause=" + strconv.FormatFloat(m.o.sentencePause, 'f', -1, 64), "lang=" + m.o.lang, "model=" + m.o.model}
		if m.dirChanged {
			sets = append(sets, "output-dir="+m.dir)
		}
		if err := updateConfig(m.o.cfg, sets, nil); err != nil {
			return fail(err)
		}
		return done(dim.Render("saved as defaults"))
	}
	return fail(fmt.Errorf("unknown command /%s, see /help", name))
}

// play writes the last audio to a temp WAV and plays it; any earlier playback stops.
func (m *tui) play() tea.Cmd {
	m.stop()
	f, err := os.CreateTemp("", "clumyap-*.wav")
	if err != nil {
		return fail(err)
	}
	err = writeWAV(f, m.last, m.eng.rate)
	f.Close()
	var c *exec.Cmd
	if err == nil {
		c, err = playerCommand(f.Name())
	}
	if err == nil {
		err = c.Start()
	}
	if err != nil {
		os.Remove(f.Name())
		return fail(err)
	}
	m.player, m.played = c, f.Name()
	return func() tea.Msg {
		c.Wait()
		os.Remove(f.Name())
		return playDoneMsg{c}
	}
}

// stop ends playback and removes its temp WAV; on exit the playback goroutine never gets to.
func (m *tui) stop() {
	if m.player != nil {
		m.player.Process.Kill()
		m.player = nil
	}
	if m.played != "" {
		os.Remove(m.played)
		m.played = ""
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
	switch {
	case m.picker != nil && !m.picker.naming:
		b.WriteString(m.pickerView() + "\n")
	case m.picker != nil:
		b.WriteString(box.BorderForeground(border).Render(m.input.View()) + "\n" + m.pickerView() + "\n")
	default:
		b.WriteString(box.BorderForeground(border).Render(m.input.View()) + "\n")
	}
	if menu := m.menu(); m.picker == nil && menu != "" {
		b.WriteString(menu + "\n")
	}
	mod, _ := findModel(m.o.model)
	sep := dim.Render(" · ")
	b.WriteString(" " + pink.Render("♪ "+m.o.voice) + sep + amber.Render(strconv.FormatFloat(m.o.speed, 'f', -1, 64)+"x") +
		sep + cyan.Render(m.o.lang) + sep + green.Render(mod.short+" model") + sep + dim.Render("📁 "+filepath.Base(m.dir)))
	if m.player != nil {
		b.WriteString(sep + pink.Render("▶ playing") + dim.Render(" esc to stop"))
	}
	return tea.NewView(b.String())
}

// menu lists the completions for a "/..." input, highlighting the one Tab will accept.
func (m *tui) menu() string {
	matches := m.matches()
	if len(matches) == 0 {
		return ""
	}
	const rows = 6
	cur := min(m.sel, len(matches)-1)
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
