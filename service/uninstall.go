package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var errUninstallSource = errors.New("this clumyap was built from source, not installed by the install script; delete the binary yourself (for example rm ~/go/bin/clumyap)")

// commandLink is the symlink install.sh puts on PATH (none on Windows, which adds the folder to PATH).
func commandLink() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin", "clumyap")
}

// modelPaths lists clumyap's own downloaded model folders. Only these, not the whole model dir,
// which CLUMYAP_MODEL_DIR may point at a folder holding other files.
func modelPaths() []string {
	base, err := modelDir()
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range models {
		if p := filepath.Join(base, m.name); exists(p) {
			out = append(out, p)
		}
	}
	return out
}

// settingsPath is the folder with config.json and lexicon.txt, or "" when there is none.
func settingsPath() string {
	cfg, err := configPath()
	if err != nil || !exists(filepath.Dir(cfg)) {
		return ""
	}
	return filepath.Dir(cfg)
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func sizeOf(paths ...string) (bytes int64) {
	for _, p := range paths {
		filepath.Walk(p, func(_ string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				bytes += info.Size()
			}
			return nil
		})
	}
	return bytes
}

// userData lists the voice models and the settings folder, with their total size.
func userData() (paths []string, bytes int64) {
	paths = modelPaths()
	if s := settingsPath(); s != "" {
		paths = append(paths, s)
	}
	return paths, sizeOf(paths...)
}

// steps receives each uninstall step as it happens, so the CLI and the TUI show the same list.
type steps interface {
	do(label, path string, fn func() error) error // a removal
	kept(label, path string)                      // something left in place on purpose
}

// uninstall removes the installed app and, with all, the models and settings too.
func uninstall(all bool, s steps) error {
	if !installedByScript() {
		return errUninstallSource
	}
	if link := commandLink(); link != "" {
		if target, err := filepath.EvalSymlinks(link); err == nil && strings.HasPrefix(target, installDir()) {
			if err := s.do("Removed the command", link, func() error { return os.Remove(link) }); err != nil {
				return err
			}
		}
	}
	dir := installDir()
	if runtime.GOOS == "windows" {
		if err := s.do("Removed clumyap from your PATH", "", func() error { return removeInstallDir(dir) }); err != nil {
			return err
		}
		s.kept("The app folder is deleted a few seconds after clumyap closes", dir)
	} else if err := s.do("Removed the app", dir, func() error { return removeInstallDir(dir) }); err != nil {
		return err
	}

	models, settings := modelPaths(), settingsPath()
	modelLabel := fmt.Sprintf("the voice models (%d MB)", sizeOf(models...)>>20)
	for _, p := range models {
		if !all {
			s.kept("Kept "+modelLabel, p)
			continue
		}
		if err := s.do("Deleted "+modelLabel, p, func() error { return os.RemoveAll(p) }); err != nil {
			return err
		}
	}
	if all {
		if base, err := modelDir(); err == nil {
			os.Remove(base) // only if now empty
		}
	}
	if settings != "" {
		if !all {
			s.kept("Kept your settings and pronunciations", settings)
		} else if err := s.do("Deleted your settings and pronunciations", settings, func() error { return os.RemoveAll(settings) }); err != nil {
			return err
		}
	}
	return nil
}

// uninstallSummary says what uninstall removes, for the confirmation.
func uninstallSummary() string {
	where := tildePath(installDir())
	if link := commandLink(); link != "" {
		where += " and " + tildePath(link)
	}
	return "This removes clumyap from " + where + "."
}

func dataSummary() string {
	_, bytes := userData()
	return fmt.Sprintf("the voice models (%d MB) and your settings and pronunciations", bytes>>20)
}

func reinstallHint() string {
	if runtime.GOOS == "windows" {
		return installPs1
	}
	return installSh
}

// cliSteps draws the steps in a terminal: a spinner while one runs, then ✓ or •.
type cliSteps struct{ fancy bool }

func (c cliSteps) line(mark, label, path string) string {
	p := ""
	if path != "" {
		p = "  " + tildePath(path)
	}
	if !c.fancy {
		return fmt.Sprintf("  %s %s%s", mark, label, p)
	}
	return "  " + mark + " " + label + dim.Render(p)
}

func (c cliSteps) do(label, path string, fn func() error) error {
	if !c.fancy {
		if err := fn(); err != nil {
			return err
		}
		fmt.Println(c.line("✓", label, path))
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- fn() }()
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	tick := time.NewTicker(80 * time.Millisecond)
	defer tick.Stop()
	fmt.Print("\x1b[?25l") // hide the cursor while drawing
	defer fmt.Print("\x1b[?25h")
	for i := 0; ; i++ {
		select {
		case err := <-done:
			fmt.Print("\r\x1b[K")
			if err != nil {
				fmt.Println(c.line(red.Render("✗"), label, path))
				return err
			}
			fmt.Println(c.line(green.Render("✓"), label, path))
			return nil
		case <-tick.C:
			fmt.Print("\r" + c.line(pink.Render(frames[i%len(frames)]), strings.Replace(label, "Deleted", "Deleting", 1), path))
		}
	}
}

func (c cliSteps) kept(label, path string) {
	mark := "•"
	if c.fancy {
		mark = cyan.Render("•")
	}
	fmt.Println(c.line(mark, label, path))
}

// runUninstall is `clumyap uninstall`: it asks before removing anything.
func runUninstall(in io.Reader) error {
	if !installedByScript() {
		return errUninstallSource
	}
	st, _ := os.Stdout.Stat()
	fancy := st != nil && st.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == ""
	paint := func(style func(...string) string, s string) string {
		if fancy {
			return style(s)
		}
		return s
	}
	r := bufio.NewReader(in)
	ask := func(q string) bool {
		fmt.Print("  " + paint(accent.Render, "?") + " " + q + paint(dim.Render, " [y/N] "))
		answer, _ := r.ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes"
	}

	fmt.Println("\n  " + paint(accent.Render, "♪ clumyap") + paint(dim.Render, " uninstaller") + "\n")
	fmt.Println("  " + uninstallSummary())
	if !ask("Remove clumyap?") {
		fmt.Println("\n  Nothing removed.")
		return nil
	}
	all := false
	if paths, _ := userData(); len(paths) > 0 {
		all = ask("Also delete " + dataSummary() + "?")
	}
	fmt.Println()
	if err := uninstall(all, cliSteps{fancy}); err != nil {
		return err
	}
	fmt.Println("\n  clumyap is uninstalled. To reinstall:")
	fmt.Println("  " + paint(dim.Render, reinstallHint()) + "\n")
	return nil
}

// tuiSteps collects the steps for /uninstall, which prints them once done.
type tuiSteps struct{ lines *[]string }

func (t tuiSteps) do(label, path string, fn func() error) error {
	if err := fn(); err != nil {
		return err
	}
	*t.lines = append(*t.lines, green.Render("✓ ")+label+dim.Render("  "+tildePath(path)))
	return nil
}

func (t tuiSteps) kept(label, path string) {
	*t.lines = append(*t.lines, cyan.Render("• ")+label+dim.Render("  "+tildePath(path)))
}
