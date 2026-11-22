package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Updates reuse the install scripts, so installing and updating stay one tested code path.
const (
	repo          = "adamwahyuh/clumyap"
	installSh     = "curl -fsSL https://raw.githubusercontent.com/" + repo + "/main/install.sh | sh"
	installPs1    = "irm https://raw.githubusercontent.com/" + repo + "/main/install.ps1 | iex"
	latestRelease = "https://api.github.com/repos/" + repo + "/releases/latest"
)

// latestVersion asks GitHub for the newest release, e.g. "0.2.0".
func latestVersion() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, latestRelease, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("can't reach GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var rel struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	return strings.TrimPrefix(rel.Tag, "v"), nil
}

// newer reports whether version a is later than b ("0.10.0" > "0.9.3"); unparsable parts count as 0.
func newer(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// installDir is where the install scripts put clumyap on this OS.
func installDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "clumyap")
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "clumyap")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "clumyap")
}

// installedByScript tells a copy from the install scripts apart from a build out of the source
// (go install), which an update must not cover up.
func installedByScript() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	dir := installDir()
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	return strings.EqualFold(filepath.Dir(exe), dir)
}

var errFromSource = errors.New("this clumyap was built from source; update it with: git pull && cd service && go install .")

// checkUpdate returns the newer version available, or "" when this one is current.
func checkUpdate() (string, error) {
	if !installedByScript() {
		return "", errFromSource
	}
	latest, err := latestVersion()
	if err != nil {
		return "", err
	}
	if !newer(latest, version()) {
		return "", nil
	}
	return latest, nil
}

// installerCommand runs the install script for this OS, which replaces the installed files.
func installerCommand() *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", installPs1)
	}
	return exec.Command("sh", "-c", installSh)
}

// runUpdate is `clumyap update`.
func runUpdate() error {
	fmt.Printf("clumyap %s, checking for updates...\n", version())
	latest, err := checkUpdate()
	switch {
	case err != nil:
		return err
	case latest == "":
		fmt.Println("Already up to date.")
		return nil
	}
	fmt.Printf("Updating to %s...\n", latest)
	cmd := installerCommand()
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
