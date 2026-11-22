package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// The personal lexicon teaches clumyap how to say words: one "word = say as" per line in
// ~/.config/clumyap/lexicon.txt. "say as" is either a respelling in ordinary words
// ("clumyap = clum yap") or Kokoro phonemes between slashes ("adam = /ˈædəm/").

var lexWordRe = regexp.MustCompile(`^[\p{L}\p{N}'-]+$`)

const userLexHeader = `# clumyap personal lexicon: how to say words.
# One entry per line:  word = say as
#   say as is ordinary words ("clumyap = clum yap") or phonemes in slashes ("adam = /ˈædəm/").
# Words match whatever their case. Lines starting with # are ignored.
`

func userLexPath() (string, error) {
	cfg, err := configPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cfg), "lexicon.txt"), nil
}

// parseLexEntry splits "word = say as"; the word is returned in lower case.
func parseLexEntry(line string) (word, sayAs string, err error) {
	word, sayAs, ok := strings.Cut(line, "=")
	word, sayAs = strings.ToLower(strings.TrimSpace(word)), strings.TrimSpace(sayAs)
	switch {
	case !ok:
		return "", "", errors.New(`write it as: word = say as`)
	case !lexWordRe.MatchString(word):
		return "", "", fmt.Errorf("%q is not a single word", word)
	case sayAs == "" || sayAs == "//":
		return "", "", fmt.Errorf("say how %q sounds after the =", word)
	}
	return word, sayAs, nil
}

// loadUserLexicon reads the personal lexicon; a missing file is an empty lexicon, bad lines are skipped.
func loadUserLexicon() (map[string]string, error) {
	lex := map[string]string{}
	path, err := userLexPath()
	if err != nil {
		return lex, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return lex, nil
	}
	if err != nil {
		return lex, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if w, s, err := parseLexEntry(line); err == nil {
			lex[w] = s
		}
	}
	return lex, sc.Err()
}

func saveUserLexicon(lex map[string]string) error {
	path, err := userLexPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	words := make([]string, 0, len(lex))
	for w := range lex {
		words = append(words, w)
	}
	sort.Strings(words)
	var b strings.Builder
	b.WriteString(userLexHeader)
	for _, w := range words {
		b.WriteString(w + " = " + lex[w] + "\n")
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// pronounce is the /pronounce command: list, add or change ("word = say as"), or remove ("-word").
func (m *tui) pronounce(arg string) tea.Cmd {
	lex, err := loadUserLexicon()
	if err != nil {
		return fail(err)
	}
	path, _ := userLexPath()
	switch {
	case arg == "":
		if len(lex) == 0 {
			return say(dim, "no words yet. Add one: /pronounce clumyap = clum yap\nfile: "+tildePath(path))
		}
		words := make([]string, 0, len(lex))
		for w := range lex {
			words = append(words, w)
		}
		sort.Strings(words)
		var b strings.Builder
		for _, w := range words {
			b.WriteString("  " + pink.Render(w) + dim.Render(" = ") + lex[w] + "\n")
		}
		return tea.Println(b.String() + dim.Render("file: "+tildePath(path)))
	case strings.HasPrefix(arg, "-"):
		w := strings.ToLower(strings.TrimSpace(arg[1:]))
		if _, ok := lex[w]; !ok {
			return fail(fmt.Errorf("%q is not in your lexicon", w))
		}
		delete(lex, w)
		if err := saveUserLexicon(lex); err != nil {
			return fail(err)
		}
		return done(dim.Render("removed ") + pink.Render(w))
	}
	w, sayAs, err := parseLexEntry(arg)
	if err != nil {
		return fail(err)
	}
	lex[w] = sayAs
	if err := saveUserLexicon(lex); err != nil {
		return fail(err)
	}
	return done(pink.Render(w) + dim.Render(" → ") + sayAs + dim.Render("  (type the word to hear it)"))
}
