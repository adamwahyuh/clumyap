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

// userData lists the voice models and the settings folder. Only clumyap's own model folders are listed,
// not the whole model dir, which CLUMYAP_MODEL_DIR may point at a folder holding other files.
func userData() (paths []string, bytes int64) {
	if base, err := modelDir(); err == nil {
		for _, m := range models {
			paths = append(paths, filepath.Join(base, m.name))
		}
	}
	if cfg, err := configPath(); err == nil {
		paths = append(paths, filepath.Dir(cfg))
	}
	var existing []string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			existing = append(existing, p)
			filepath.Walk(p, func(_ string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					bytes += info.Size()
				}
				return nil
			})
		}
	}
	return existing, bytes
}

// uninstall removes the installed app and, with all, the models and settings too.
func uninstall(all bool) error {
	if !installedByScript() {
		return errUninstallSource
	}
	if all {
		paths, _ := userData()
		for _, p := range paths {
			if err := os.RemoveAll(p); err != nil {
				return err
			}
		}
		if base, err := modelDir(); err == nil {
			os.Remove(base) // only if now empty
		}
	}
	if link := commandLink(); link != "" {
		if target, err := filepath.EvalSymlinks(link); err == nil && strings.HasPrefix(target, installDir()) {
			if err := os.Remove(link); err != nil {
				return err
			}
		}
	}
	return removeInstallDir(installDir())
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

// runUninstall is `clumyap uninstall`: it asks before removing anything.
func runUninstall(in io.Reader) error {
	if !installedByScript() {
		return errUninstallSource
	}
	r := bufio.NewReader(in)
	ask := func(q string) bool {
		fmt.Print(q + " [y/N] ")
		answer, _ := r.ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes"
	}
	fmt.Println(uninstallSummary())
	if !ask("Remove clumyap?") {
		fmt.Println("Nothing removed.")
		return nil
	}
	all := ask("Also delete " + dataSummary() + "?")
	if err := uninstall(all); err != nil {
		return err
	}
	if all {
		fmt.Println("clumyap and its data were removed.")
	} else {
		paths, _ := userData()
		for i := range paths {
			paths[i] = tildePath(paths[i])
		}
		fmt.Println("clumyap removed. Models and settings were kept, in: " + strings.Join(paths, ", "))
	}
	return nil
}
