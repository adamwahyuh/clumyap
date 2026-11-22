package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	pp "github.com/csukuangfj/piper-phonemize-go/piper_phonemize"
)

func TestPhonemizeKeepsPauses(t *testing.T) {
	home, _ := os.UserHomeDir()
	data := filepath.Join(home, ".cache/clumyap", models[0].name, "espeak-ng-data")
	if _, err := os.Stat(data); err != nil {
		t.Skip("model not downloaded")
	}
	pp.Initialize(data)
	for _, in := range []string{"Wait... really?", "Wait…really?", "Hmm...okay."} {
		var all string
		for _, s := range phonemize(in, "en-us") {
			all += string(s)
		}
		if !strings.Contains(all, ",") || !strings.Contains(all, " ") {
			t.Errorf("%q lost its pause: %q", in, all)
		}
	}
}
