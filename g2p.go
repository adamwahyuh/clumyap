package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	pp "github.com/csukuangfj/piper-phonemize-go/piper_phonemize"
)

// g2p turns English text into Kokoro phonemes the way Kokoro's own pipeline (misaki) does:
// the official lexicon first, espeak-ng for anything else, converted to Kokoro's symbols.
type g2p struct {
	lex     map[string]string
	voice   string // espeak voice
	british bool
}

// maxChunk keeps each model call under Kokoro's 510-token limit while letting several
// sentences share one call, so intonation flows across them instead of resetting.
const maxChunk = 400

func newG2P(modelDir, lang string) (*g2p, error) {
	file, voice := "lexicon-us-en.txt", "en-us"
	if lang == "en-gb" {
		file, voice = "lexicon-gb-en.txt", "en-gb-x-rp"
	}
	f, err := os.Open(filepath.Join(modelDir, file))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	lex := make(map[string]string, 180000)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) > 1 {
			lex[fields[0]] = strings.Join(fields[1:], "")
		}
	}
	return &g2p{lex: lex, voice: voice, british: lang == "en-gb"}, sc.Err()
}

const punct = `;:,.!?—…"()“”`

var (
	normalizer = strings.NewReplacer("’", "'", "‘", "'", "...", " … ", "…", " … ", "—", " — ", " - ", " — ", " – ", " — ")
	sentEnd    = regexp.MustCompile(`[.!?…]["”)]*$`)
	syllabic   = regexp.MustCompile(`(\S)\x{0329}`)
)

// chunks returns the paragraph as phoneme strings of at most maxChunk runes, split at sentence ends when possible.
func (g *g2p) chunks(text string) []string {
	var sentences []string
	var cur []string
	for _, tok := range strings.Fields(normalizer.Replace(text)) {
		lead, core, trail := splitPunct(tok)
		ph := keepPunct(lead) + g.word(core) + keepPunct(trail)
		switch {
		case ph == "":
		case core == "" && len(cur) > 0 && (strings.ContainsAny(ph[:1], `;:,.!?"”)`) || strings.HasPrefix(ph, "…")):
			cur[len(cur)-1] += ph // closing punctuation sticks to the previous word: "jˈɛs, aɪ"; dashes stay spaced
		default:
			cur = append(cur, ph)
		}
		if sentEnd.MatchString(tok) && len(cur) > 0 {
			sentences = append(sentences, strings.Join(cur, " "))
			cur = nil
		}
	}
	if len(cur) > 0 {
		sentences = append(sentences, strings.Join(cur, " "))
	}

	var out []string
	chunk := ""
	flush := func() {
		if chunk != "" {
			out = append(out, chunk)
			chunk = ""
		}
	}
	for _, s := range sentences {
		if chunk != "" && utf8.RuneCountInString(chunk)+1+utf8.RuneCountInString(s) <= maxChunk {
			chunk += " " + s
			continue
		}
		flush()
		// A single over-long sentence is split between words.
		for _, w := range strings.Fields(s) {
			if chunk != "" && utf8.RuneCountInString(chunk)+1+utf8.RuneCountInString(w) > maxChunk {
				flush()
			}
			if chunk != "" {
				chunk += " "
			}
			chunk += w
		}
	}
	flush()
	return out
}

func splitPunct(tok string) (lead, core, trail string) {
	core = strings.TrimLeft(tok, punct)
	lead = tok[:len(tok)-len(core)]
	trimmed := strings.TrimRight(core, punct)
	return lead, trimmed, core[len(trimmed):]
}

func keepPunct(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(punct, r) {
			return r
		}
		return -1
	}, s)
}

func (g *g2p) word(w string) string {
	if w == "" {
		return ""
	}
	lower := strings.ToLower(w)
	if ph, ok := g.lex[lower]; ok {
		return ph
	}
	if parts := strings.Split(lower, "-"); len(parts) > 1 {
		var phs []string
		for _, p := range parts {
			if p != "" {
				phs = append(phs, g.word(p))
			}
		}
		return strings.Join(phs, " ")
	}
	return g.espeak(w)
}

// e2m mirrors misaki's EspeakFallback (sequential, longest first). piper-phonemize emits no '^' ties,
// so the tied forms are written without them; 'ə^l'->'ᵊl' is skipped because untied it would also hit "a lot".
var e2m = []string{
	"ʔˌn̩", "ʔn", "ʔn̩", "ʔn",
	"aɪ", "I", "aʊ", "W", "dʒ", "ʤ", "eɪ", "A", "tʃ", "ʧ", "ɔɪ", "Y",
	"ʲo", "jo", "ʲə", "jə", "ʲ", "",
	"e", "A", "ɚ", "əɹ", "r", "ɹ", "x", "k", "ç", "k", "ɐ", "ə", "ɬ", "l", "̃", "",
}

var (
	e2mUS = []string{"oʊ", "O", "ɜːɹ", "ɜɹ", "ɜː", "ɜɹ", "ɪə", "iə", "ː", ""}
	e2mGB = []string{"eə", "ɛː", "iə", "ɪə", "əʊ", "Q"}
)

func toKokoro(ps string, british bool) string {
	for i := 0; i < len(e2m); i += 2 {
		ps = strings.ReplaceAll(ps, e2m[i], e2m[i+1])
	}
	ps = strings.ReplaceAll(syllabic.ReplaceAllString(ps, "ᵊ$1"), "̩", "")
	rules := e2mUS
	if british {
		rules = e2mGB
	}
	for i := 0; i < len(rules); i += 2 {
		ps = strings.ReplaceAll(ps, rules[i], rules[i+1])
	}
	return strings.ReplaceAll(ps, "o", "ɔ")
}

func (g *g2p) espeak(w string) string {
	r := pp.Phonemize(w, g.voice)
	defer pp.DeletePhonemizeResult(r)
	var parts []string
	for s := 0; s < r.GetNumSentences(); s++ {
		var b strings.Builder
		for _, c := range r.GetPhonemes(s) {
			b.WriteRune(rune(c))
		}
		parts = append(parts, strings.TrimSpace(b.String()))
	}
	return toKokoro(keepPhonemes(strings.Join(parts, " ")), g.british)
}

// keepPhonemes drops punctuation espeak may echo for a bare token; the caller places punctuation itself.
func keepPhonemes(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(punct, r) {
			return -1
		}
		return r
	}, s)
}
