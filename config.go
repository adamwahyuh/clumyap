package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var settingKeys = []string{"voice", "model", "speed", "lang", "output-dir"}

var settingDefaults = map[string]string{
	"voice": "af_heart", "model": models[0].name, "speed": "1.0", "lang": "en-us", "output-dir": "",
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "clumyap", "config.json"), nil
}

// normalizeSetting validates a value and returns the form to store.
func normalizeSetting(key, val string) (string, error) {
	switch key {
	case "voice":
		if id := voiceID(val); id >= 0 {
			return voices[id], nil
		}
		return "", fmt.Errorf("unknown voice %q, see --voices", val)
	case "model":
		if isModel(val) {
			return val, nil
		}
		return "", fmt.Errorf("unknown model %q, see --models", val)
	case "speed":
		if f, err := strconv.ParseFloat(val, 64); err == nil && f >= 0.5 && f <= 2 {
			return strconv.FormatFloat(f, 'f', -1, 64), nil
		}
		return "", fmt.Errorf("speed must be between 0.5 and 2.0")
	case "lang":
		if val == "en-us" || val == "en-gb" {
			return val, nil
		}
		return "", fmt.Errorf("lang must be en-us or en-gb")
	case "output-dir":
		if val == "" {
			return "", fmt.Errorf("output-dir is empty, use --unset output-dir instead")
		}
		return expandPath(val)
	}
	return "", fmt.Errorf("unknown setting %q (settings: %s)", key, strings.Join(settingKeys, ", "))
}

// expandPath turns "~/x" and relative paths into absolute ones.
func expandPath(p string) (string, error) {
	if rest, ok := strings.CutPrefix(p, "~"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = home + rest
	}
	return filepath.Abs(p)
}

// loadConfig returns the saved settings; a missing file means none, invalid entries are dropped with a warning.
func loadConfig() (map[string]string, error) {
	cfg := map[string]string{}
	path, err := configPath()
	if err != nil {
		return cfg, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return map[string]string{}, fmt.Errorf("broken config file %s: %v", path, err)
	}
	for k, v := range cfg {
		if _, err := normalizeSetting(k, v); err != nil {
			fmt.Fprintf(os.Stderr, "clumyap: ignoring setting %s=%q in %s: %v\n", k, v, path, err)
			delete(cfg, k)
		}
	}
	return cfg, nil
}

func saveConfig(cfg map[string]string) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// updateConfig applies "key=value" sets and key unsets; nothing is saved if any of them is invalid.
func updateConfig(cfg map[string]string, sets, unsets []string) error {
	for _, kv := range sets {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return fmt.Errorf("%w: --set needs key=value, got %q", errUsage, kv)
		}
		norm, err := normalizeSetting(k, v)
		if err != nil {
			return fmt.Errorf("%w: %v", errUsage, err)
		}
		cfg[k] = norm
	}
	for _, k := range unsets {
		if _, ok := settingDefaults[k]; !ok {
			return fmt.Errorf("%w: unknown setting %q (settings: %s)", errUsage, k, strings.Join(settingKeys, ", "))
		}
		delete(cfg, k)
	}
	return saveConfig(cfg)
}

func printConfig(cfg map[string]string) {
	path, _ := configPath()
	fmt.Println("config file:", path)
	for _, k := range settingKeys {
		v, ok := cfg[k]
		switch {
		case ok:
		case k == "output-dir":
			v = "(default: current folder)"
		default:
			v = settingDefaults[k] + " (default)"
		}
		fmt.Printf("  %-11s %s\n", k, v)
	}
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(s string) error { *m = append(*m, s); return nil }
