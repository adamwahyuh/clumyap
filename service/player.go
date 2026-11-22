package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// playerCommand returns a command that plays the WAV file at path and exits when it is done;
// killing the process stops playback. Every player used ships with the OS or its usual audio stack.
func playerCommand(path string) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("afplay", path), nil
	case "windows":
		// System.Media.SoundPlayer is part of every Windows install and plays WAV.
		script := fmt.Sprintf("(New-Object Media.SoundPlayer '%s').PlaySync()", strings.ReplaceAll(path, "'", "''"))
		return exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script), nil
	}
	for _, p := range [][]string{{"pw-play"}, {"paplay"}, {"aplay", "-q"}, {"ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet"}} {
		if _, err := exec.LookPath(p[0]); err == nil {
			return exec.Command(p[0], append(p[1:], path)...), nil
		}
	}
	return nil, errors.New("no audio player found: install pipewire, pulseaudio-utils or alsa-utils")
}

// playSamples plays audio through a temporary WAV and waits until it ends.
func playSamples(samples []float32, rate int) error {
	f, err := os.CreateTemp("", "clumyap-*.wav")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = writeWAV(f, samples, rate)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	cmd, err := playerCommand(f.Name())
	if err != nil {
		return err
	}
	return cmd.Run()
}
