package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	pp "github.com/csukuangfj/piper-phonemize-go/piper_phonemize"
)

func TestToKokoro(t *testing.T) {
	for in, want := range map[string]string{
		"wˈeɪt":    "wˈAt",  // eɪ -> A
		"hɛlˈoʊ":   "hɛlˈO", // oʊ -> O (US)
		"dʒˈɑːb":   "ʤˈɑb",  // dʒ -> ʤ, length mark dropped (US)
		"bˈɜːd":    "bˈɜɹd",
		"fˈɚst":    "fˈəɹst",
		"maɪ rˈɔɪ": "mI ɹˈY",
	} {
		if got := toKokoro(in, false); got != want {
			t.Errorf("toKokoro(%q) = %q, want %q", in, got, want)
		}
	}
	if got := toKokoro("hɛlˈəʊ", true); got != "hɛlˈQ" {
		t.Errorf("british əʊ: %q", got)
	}
}

func TestChunks(t *testing.T) {
	g := &g2p{lex: map[string]string{"yes": "jˈɛs", "i": "ˈI", "did": "dˈɪd", "it": "ɪt", "wait": "wˈAt", "really": "ɹˈiᵊli", "well-known": "wˈɛlnˈOn"}}
	if got := g.chunks(`Yes, I did it! "Wait... really?"`); len(got) != 1 || got[0] != `jˈɛs, ˌI dˈɪd ɪt! "wˈAt… ɹˈiᵊli?"` {
		t.Fatalf("got %q", got)
	}
	if got := g.chunks("Well-known — yes."); got[0] != "wˈɛlnˈOn — jˈɛs." {
		t.Fatalf("got %q", got)
	}
	// Leading punctuation with no word before it must not panic.
	if got := g.chunks("... yes"); len(got) != 1 {
		t.Fatalf("leading ellipsis: %q", got)
	}
	// Many sentences pack into chunks under the limit, split only at sentence ends.
	long := strings.Repeat("Yes, I did it. ", 60)
	chunks := g.chunks(long)
	if len(chunks) < 2 {
		t.Fatalf("want several chunks, got %d", len(chunks))
	}
	for _, c := range chunks {
		if utf8.RuneCountInString(c) > maxChunk || !strings.HasSuffix(c, ".") {
			t.Fatalf("bad chunk (%d runes): %q", utf8.RuneCountInString(c), c)
		}
	}
	// One endless sentence still splits, between words.
	for _, c := range g.chunks(strings.Repeat("yes ", 300)) {
		if utf8.RuneCountInString(c) > maxChunk {
			t.Fatalf("chunk over limit: %d", utf8.RuneCountInString(c))
		}
	}
}

func TestG2PWithModel(t *testing.T) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".cache/clumyap", models[0].name)
	if _, err := os.Stat(filepath.Join(dir, "lexicon-us-en.txt")); err != nil {
		t.Skip("model not downloaded")
	}
	pp.Initialize(filepath.Join(dir, "espeak-ng-data"))
	g, err := newG2P(dir, "en-us")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(g.chunks("Hello! I deployed 25 apps to GitHub."), " ")
	for _, want := range []string{"həlˈO!", "dᵻplˈYd", "twˈɛnti fˈIv", "ɡˈɪt hˈʌb."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	// Only Kokoro's symbols may reach the model.
	tokens, _ := os.ReadFile(filepath.Join(dir, "tokens.txt"))
	known := map[rune]bool{' ': true}
	for _, line := range strings.Split(string(tokens), "\n") {
		if r, _ := utf8.DecodeRuneInString(line); line != "" {
			known[r] = true
		}
	}
	for _, r := range got {
		if !known[r] {
			t.Errorf("symbol %q is not a Kokoro token (in %q)", r, got)
		}
	}
}

func TestSentencePause(t *testing.T) {
	if !modelInstalled(models[0]) {
		t.Skip("model not downloaded")
	}
	e, err := loadEngine(models[0])
	if err != nil {
		t.Fatal(err)
	}
	defer e.close()
	o, _ := parseArgs(nil, map[string]string{})
	text := "Hello there. How are you? I am fine."
	flow, err := e.synth(text, o, nil)
	if err != nil {
		t.Fatal(err)
	}
	o.sentencePause = 1
	paused, err := e.synth(text, o, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Three sentences give two gaps of the asked 1 s (plus the 2x40 ms trim margins); flowing speech has none that long.
	if g := silentGaps(flow, e.rate); len(g) != 0 {
		t.Errorf("flowing speech should have no long gaps, got %v", g)
	}
	g := silentGaps(paused, e.rate)
	if len(g) != 2 || g[0] < 1 || g[0] > 1.15 || g[1] < 1 || g[1] > 1.15 {
		t.Fatalf("want two ~1s gaps, got %v", g)
	}
	if got := trimSilence([]float32{0, 0, 0.5, 0, 0}, 1000); len(got) != 5 {
		t.Fatalf("trim must keep a margin: %v", got)
	}
	if got := trimSilence(make([]float32, 1000), 1000); len(got) > 80 {
		t.Fatalf("all-silent input should shrink to the margin, got %d samples", len(got))
	}
	t.Logf("flow %.2fs, with 1s sentence pause %.2fs", float64(len(flow))/float64(e.rate), float64(len(paused))/float64(e.rate))
}

// silentGaps returns the length in seconds of every inner silent run longer than 0.3 s.
func silentGaps(s []float32, rate int) []float64 {
	var out []float64
	run := 0
	for _, v := range s {
		if v < 0.01 && v > -0.01 {
			run++
			continue
		}
		if run > rate*3/10 {
			out = append(out, float64(run)/float64(rate))
		}
		run = 0
	}
	return out
}

func TestNaturalReading(t *testing.T) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".cache/clumyap", models[0].name)
	if _, err := os.Stat(filepath.Join(dir, "lexicon-us-en.txt")); err != nil {
		t.Skip("model not downloaded")
	}
	pp.Initialize(filepath.Join(dir, "espeak-ng-data"))
	g, err := newG2P(dir, "en-us")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		text      string
		sentences int
		has       []string
		hasNot    []string
	}{
		// Weak forms: "the" is "thuh" before a consonant and "thee" before a vowel; "a"/"an" are "uh"/"un".
		{"The cat ate the apple. A dog and an owl.", 2, []string{"ðə kˈæt", "ði ˈæpᵊl", "ɐ dˈɔɡ", "ɐn ˈWl"}, []string{"ði kˈæt", "A dˈɔɡ"}},
		{"I want to go to Oslo. I am here.", 2, []string{"ˌI wˈɑnt", "tə ɡ", "tʊ ˈɑ", "ɐm hˈɪɹ"}, nil},
		{"Plan A.", 1, []string{"ˈA."}, nil},
		// Abbreviations read in full and do not end the sentence.
		{"Dr. Smith met Mr. Jones at 3:30 p.m. on Jan. 5th.", 1, []string{"dˈɑktəɹ smˈɪθ", "mˈɪstəɹ ʤˈOnz", "ʤˈænjə"}, []string{"dˈɑktəɹ.", "mˈɪstəɹ."}},
		{"Red vs. blue, e.g. this one.", 1, []string{"vˈɜɹsəs blˈu"}, nil},
		{"J. K. Rowling wrote it.", 1, nil, nil},
		// A real full stop after an abbreviation-like word still ends the sentence.
		{"We met in Jan. Then it rained.", 2, nil, nil},
		{"Apples, pears, etc. Then lunch.", 2, nil, nil},
	}
	for _, c := range cases {
		got := g.sentences(c.text)
		all := strings.Join(got, " | ")
		if len(got) != c.sentences {
			t.Errorf("%q: %d sentences, want %d: %s", c.text, len(got), c.sentences, all)
		}
		for _, w := range c.has {
			if !strings.Contains(all, w) {
				t.Errorf("%q: missing %q in %s", c.text, w, all)
			}
		}
		for _, w := range c.hasNot {
			if strings.Contains(all, w) {
				t.Errorf("%q: should not contain %q: %s", c.text, w, all)
			}
		}
	}
}
