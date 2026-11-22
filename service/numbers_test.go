package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeNumbers(t *testing.T) {
	for in, want := range map[string]string{
		"It costs $4.99 now.":     "It costs four dollars and ninety-nine cents now.",
		"$1 and $0.99 and $4.5":   "one dollar and ninety-nine cents and four dollars and fifty cents",
		"$1,250 total":            "one thousand two hundred fifty dollars total",
		"$2.5 million deal":       "two point five million dollars deal",
		"£5.20 or €1":             "five pounds and twenty pence or one euro",
		"In 1999 and 2026.":       "In nineteen ninety-nine and twenty twenty-six.",
		"1900, 1905, 2000, 2005":  "nineteen hundred, nineteen oh-five, two thousand, two thousand five",
		"the 1990s and 2010s":     "the nineteen nineties and twenty tens",
		"1,999 or 19.99 or 1999%": "1,999 or 19.99 or 1999%", // not years: left to espeak
		"25 apps at 3:30, 2026px": "25 apps at 3:30, 2026px",
	} {
		if got := normalizeNumbers(in); got != want {
			t.Errorf("normalizeNumbers(%q)\n got  %q\n want %q", in, got, want)
		}
	}
	for n, want := range map[int64]string{0: "zero", 21: "twenty-one", 100: "one hundred", 1001: "one thousand one", 2_000_015: "two million fifteen"} {
		if got := numWords(n); got != want {
			t.Errorf("numWords(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestUserLexicon(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if lex, err := loadUserLexicon(); err != nil || len(lex) != 0 {
		t.Fatalf("missing file should be an empty lexicon: %v %v", lex, err)
	}
	if err := saveUserLexicon(map[string]string{"clumyap": "clum yap", "adam": "/ˈædəm/"}); err != nil {
		t.Fatal(err)
	}
	path, _ := userLexPath()
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("not an entry\ntwo words = x\nKokoro = KO ko ro\n")
	f.Close()
	lex, _ := loadUserLexicon()
	want := map[string]string{"clumyap": "clum yap", "adam": "/ˈædəm/", "kokoro": "KO ko ro"}
	if !reflect.DeepEqual(lex, want) {
		t.Fatalf("got %v, want %v (bad lines skipped, words lower-cased)", lex, want)
	}

	// The personal entry wins over the official lexicon, in any case; respellings use the official one.
	g := &g2p{lex: map[string]string{"adam": "ˈADəm", "said": "sˈɛd", "clum": "klˈʌm", "yap": "jˈæp"}, user: lex}
	if got := strings.Join(g.sentences("Adam said Clumyap."), " "); got != "ˈædəm sˈɛd klˈʌm jˈæp." {
		t.Errorf("personal lexicon not used: %q", got)
	}
}

func TestPronounceCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	o, _ := parseArgs(nil, map[string]string{})
	m := newTUI(o)
	for _, c := range []string{"/pronounce", "/pronounce Clumyap = clum yap", "/pronounce oops", "/pronounce two words = x", "/pronounce -nothere"} {
		if m.command(c) == nil {
			t.Errorf("%s: want a message", c)
		}
	}
	if lex, _ := loadUserLexicon(); lex["clumyap"] != "clum yap" || len(lex) != 1 {
		t.Fatalf("after add: %v", lex)
	}
	m.command("/pronounce -CLUMYAP")
	if lex, _ := loadUserLexicon(); len(lex) != 0 {
		t.Fatalf("after remove: %v", lex)
	}
	if _, err := os.Stat(filepath.Join(home, ".config/clumyap/lexicon.txt")); err != nil {
		t.Fatal("lexicon file should exist next to config.json")
	}
}
