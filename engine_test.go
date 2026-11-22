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
	if got := g.chunks(`Yes, I did it! "Wait... really?"`); len(got) != 1 || got[0] != `jˈɛs, ˈI dˈɪd ɪt! "wˈAt… ɹˈiᵊli?"` {
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
