package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	pp "github.com/csukuangfj/piper-phonemize-go/piper_phonemize"
)

// g2p turns English text into Kokoro phonemes the way Kokoro's own pipeline (misaki) does:
// the official lexicon first, espeak-ng for anything else, converted to Kokoro's symbols.
type g2p struct {
	lex     map[string]string
	user    map[string]string            // personal lexicon, lower-case word -> say-as; wins over everything
	het     map[string]map[string]string // words said differently by word class ("read", "live", "record")
	voice   string                       // espeak voice
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
	het := heteronymsUS
	if lang == "en-gb" {
		het = heteronymsGB
	}
	return &g2p{lex: lex, het: het, voice: voice, british: lang == "en-gb"}, sc.Err()
}

const punct = `;:,.!?—…"()“”`

var (
	normalizer    = strings.NewReplacer("’", "'", "‘", "'", "...", " … ", "…", " … ", "—", " — ", " - ", " — ", " – ", " — ")
	sentEnd       = regexp.MustCompile(`[.!?…]["”)]*$`)
	initialism    = regexp.MustCompile(`^(?:[A-Za-z]{1,2}\.)+[A-Za-z]{1,2}$`) // "p.m", "e.g", "U.S" (trailing dot already split off)
	singleInitial = regexp.MustCompile(`^[A-Z]$`)
	syllabic      = regexp.MustCompile(`(\S)\x{0329}`)
)

// chunks returns the paragraph as phoneme strings of at most maxChunk runes, split at sentence ends when possible.
func (g *g2p) chunks(text string) []string { return pack(g.sentences(text)) }

type token struct {
	raw               string // as written
	lead, core, trail string // punctuation before, the word, punctuation after
	abbrev            bool   // core was written as an abbreviation ("Dr.", "p.m.", "J.")
	title             bool   // an abbreviation that always precedes a name ("Dr.", "Mr.")
	ph                string // phonemes of core
}

// Abbreviations read in full. Titles never end a sentence: they always precede a name.
var (
	titleAbbrevs = map[string]string{"dr": "doctor", "mr": "mister", "mrs": "missus", "ms": "miz", "prof": "professor", "jr": "junior", "sr": "senior"}
	otherAbbrevs = map[string]string{"vs": "versus", "etc": "et cetera", "approx": "approximately", "dept": "department", "fig": "figure",
		"jan": "january", "feb": "february", "apr": "april", "jun": "june", "jul": "july", "aug": "august",
		"sep": "september", "sept": "september", "oct": "october", "nov": "november", "dec": "december"}
)

// sentences returns the paragraph as one phoneme string per sentence.
func (g *g2p) sentences(text string) []string {
	var toks []token
	for _, raw := range strings.Fields(normalizer.Replace(normalizeNumbers(text))) {
		t := token{raw: raw}
		t.lead, t.core, t.trail = splitPunct(raw)
		lower, dotted := strings.ToLower(t.core), strings.HasPrefix(t.trail, ".")
		switch {
		case dotted && titleAbbrevs[lower] != "":
			t.core, t.abbrev, t.title = titleAbbrevs[lower], true, true
		case otherAbbrevs[lower] != "" && (dotted || lower == "vs"):
			t.core, t.abbrev = otherAbbrevs[lower], true
		case dotted && (initialism.MatchString(t.core) || singleInitial.MatchString(t.core)):
			t.abbrev = true // "p.m.", "e.g.", "U.S.", "J."
		}
		toks = append(toks, t)
	}

	pastCue := pastCueRe.MatchString(text)

	// Right to left, so each word knows how the next one starts (misaki's future_vowel).
	future := fNone
	for i := len(toks) - 1; i >= 0; i-- {
		t := &toks[i]
		var parts []string
		for _, w := range strings.Fields(t.core) {
			parts = append(parts, g.word(w))
		}
		t.ph = strings.Join(parts, " ")
		if strings.ContainsAny(t.trail, nonQuotePunct) {
			future = fNone
		}
		if _, mine := g.user[strings.ToLower(t.core)]; !mine {
			if forms := g.het[strings.ToLower(t.core)]; forms != nil && !t.abbrev {
				prev, next, nextIsPunct := neighbours(toks, i)
				t.ph = pickForm(forms, wordClass(prev, next, nextIsPunct, pastCue), future)
			}
			t.ph = reduced(t.core, t.ph, future, g)
		}
		future = startOf(t.ph, future)
		if strings.ContainsAny(t.lead, nonQuotePunct) {
			future = fNone
		}
	}

	var sentences []string
	var cur []string
	for i, t := range toks {
		trail := t.trail
		end := sentEnd.MatchString(t.raw)
		if end && strings.HasPrefix(trail, ".") && !strings.HasPrefix(trail, "…") && i+1 < len(toks) && continues(t, toks[i+1]) {
			end, trail = false, trail[1:] // "Dr. Smith", "p.m. on": the dot is not a full stop
		}
		ph := keepPunct(t.lead) + t.ph + keepPunct(trail)
		switch {
		case ph == "":
		case t.core == "" && len(cur) > 0 && (strings.ContainsAny(ph[:1], `;:,.!?"”)`) || strings.HasPrefix(ph, "…")):
			cur[len(cur)-1] += ph // closing punctuation sticks to the previous word: "jˈɛs, aɪ"; dashes stay spaced
		default:
			cur = append(cur, ph)
		}
		if end && len(cur) > 0 {
			sentences = append(sentences, strings.Join(cur, " "))
			cur = nil
		}
	}
	if len(cur) > 0 {
		sentences = append(sentences, strings.Join(cur, " "))
	}
	return sentences
}

// continues reports whether the sentence goes on after t's trailing dot.
func continues(t, next token) bool {
	r, _ := utf8.DecodeRuneInString(strings.TrimLeft(next.lead+next.core, punct))
	switch {
	case unicode.IsLower(r):
		return true // nothing starts a sentence in lower case: "approx. five", "p.m. on"
	case t.title:
		return true // "Dr. Smith"
	case t.abbrev:
		return unicode.IsDigit(r) || singleInitialCore(t) // "Jan. 5th", "J. K. Rowling"
	}
	return false
}

func singleInitialCore(t token) bool { return singleInitial.MatchString(t.core) }

// future describes how the next word starts, as misaki tracks it.
type futureKind int

const (
	fNone futureKind = iota // punctuation or end of text follows
	fVowel
	fConsonant
)

const (
	kokoroVowels     = "AIOQWYaiuæɑɒɔəɛɜɪʊʌᵻ"
	kokoroConsonants = "bdfhjklmnpstvwzðŋɡɹɾʃʒʤʧθ"
	nonQuotePunct    = ";:,.!?—…"
)

func startOf(ph string, otherwise futureKind) futureKind {
	for _, r := range ph {
		switch {
		case strings.ContainsRune(kokoroVowels, r):
			return fVowel
		case strings.ContainsRune(kokoroConsonants, r):
			return fConsonant
		case strings.ContainsRune(nonQuotePunct, r):
			return fNone
		}
	}
	return otherwise
}

// reduced gives the weak forms native speakers use for short function words (misaki's get_special_case);
// the lexicon alone always has the stressed form ("the" as "thee", "a" as "ay").
func reduced(word, ph string, next futureKind, g *g2p) string {
	switch word {
	case "the", "The":
		if next == fVowel {
			return "ði"
		}
		return "ðə"
	case "a":
		return "ɐ"
	case "A":
		if next == fNone {
			return "ˈA" // the letter, as in "plan A."
		}
		return "ɐ"
	case "an", "An":
		return "ɐn"
	case "am", "Am":
		if next == fNone {
			return ph
		}
		return "ɐm"
	case "I":
		return "ˌI"
	case "to", "To":
		switch next {
		case fVowel:
			return "tʊ"
		case fConsonant:
			return "tə"
		}
		return ph
	case "in", "In":
		if next == fNone {
			return "ˈɪn"
		}
		return "ɪn"
	}
	return ph
}

// pack joins sentences into strings of at most maxChunk runes. A sentence too long for one model
// call is cut where a speaker would pause anyway (after , ; : or at a dash), and only between plain
// words when a single clause is still too long.
func pack(sentences []string) []string {
	var pieces []string
	for _, s := range sentences {
		pieces = append(pieces, pauses(s)...)
	}
	var out []string
	chunk := ""
	for _, p := range pieces {
		if chunk != "" && utf8.RuneCountInString(chunk)+1+utf8.RuneCountInString(p) > maxChunk {
			out = append(out, chunk)
			chunk = ""
		}
		if chunk != "" {
			chunk += " "
		}
		chunk += p
	}
	if chunk != "" {
		out = append(out, chunk)
	}
	return out
}

// pauses splits an over-long sentence into clauses, and any clause still over maxChunk into words.
func pauses(s string) []string {
	if utf8.RuneCountInString(s) <= maxChunk {
		return []string{s}
	}
	var clauses []string
	cur := ""
	for _, w := range strings.Fields(s) {
		if cur != "" {
			cur += " "
		}
		cur += w
		if w == "—" || strings.ContainsAny(w[len(w)-1:], ",;:") {
			clauses = append(clauses, cur)
			cur = ""
		}
	}
	if cur != "" {
		clauses = append(clauses, cur)
	}
	var out []string
	for _, c := range clauses {
		if utf8.RuneCountInString(c) <= maxChunk {
			out = append(out, c)
		} else {
			out = append(out, strings.Fields(c)...)
		}
	}
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
	if say, ok := g.user[lower]; ok {
		if ph, ok := strings.CutPrefix(say, "/"); ok { // phonemes written by hand
			return strings.TrimSuffix(ph, "/")
		}
		var phs []string
		for _, part := range strings.Fields(say) { // a respelling: read its words without the personal lexicon
			phs = append(phs, (&g2p{lex: g.lex, voice: g.voice, british: g.british}).word(part))
		}
		return strings.Join(phs, " ")
	}
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

// Heteronyms: Kokoro's pipeline picks "read" (reed/red) or "live" (liv/laiv) with a part-of-speech
// tagger. We have none, so the word before decides, which covers the common patterns.
var (
	pastCueRe   = regexp.MustCompile(`(?i)\b(yesterday|ago|last|earlier)\b`)
	toVerb      = setOf("to", "will", "would", "can", "could", "shall", "should", "may", "might", "must", "do", "does", "did", "don't", "doesn't", "didn't", "won't", "can't", "couldn't", "wouldn't", "shouldn't", "let's", "please")
	toPerfect   = setOf("have", "has", "had", "having", "i've", "you've", "we've", "they've")
	toBe        = setOf("is", "are", "was", "were", "be", "been", "being", "am", "it's", "that's")
	thirdPerson = setOf("he", "she", "it")
	subjects    = setOf("i", "you", "we", "they")
	determiners = setOf("the", "a", "an", "this", "that", "these", "those", "my", "your", "his", "her", "its", "our", "their", "every", "each", "no")
	afterNoun   = setOf("of", "is", "was", "are", "were", "in", "on", "at", "for", "with", "from", "to")
	objects     = setOf("it", "this", "that", "the", "a", "an", "your", "my", "them", "me", "us", "his", "her", "our", "their")
)

func setOf(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

// neighbours returns the words around toks[i] within its clause ("" across punctuation or the text's edge).
func neighbours(toks []token, i int) (prev, next string, nextIsPunct bool) {
	if i > 0 && !strings.ContainsAny(toks[i-1].trail+toks[i].lead, nonQuotePunct) {
		prev = strings.ToLower(toks[i-1].core)
	}
	if i+1 >= len(toks) || strings.ContainsAny(toks[i].trail+toks[i+1].lead, nonQuotePunct) {
		return prev, "", true
	}
	return prev, strings.ToLower(toks[i+1].core), false
}

// wordClass guesses misaki's tag for a heteronym from the words around it.
func wordClass(prev, next string, nextIsPunct, pastCue bool) string {
	switch {
	case toVerb[prev]:
		return "VB" // "want to record", "don't object"
	case toPerfect[prev]:
		return "VBN" // "have read"
	case toBe[prev]:
		return "BE" // "was read" (past participle) or "is close" (adjective)
	case thirdPerson[prev]:
		return "VBD" // "he read": the present would be "reads"
	case subjects[prev]:
		if pastCue {
			return "VBD" // "I read it yesterday"
		}
		return "VB" // "we live here"
	case determiners[prev]:
		if nextIsPunct || afterNoun[next] {
			return "NN" // "the use of", "the content is"
		}
		return "DEFAULT" // "the close friend", "a minute ago": usually describing what follows
	case prev == "" && objects[next]:
		return "VB" // a command: "Record it", "Close the door"
	}
	return "DEFAULT"
}

func pickForm(forms map[string]string, class string, next futureKind) string {
	if p := forms["None"]; next == fNone && p != "" {
		return p // misaki has a few words said differently before a pause
	}
	if class == "BE" {
		if p := forms["VBN"]; p != "" {
			return p
		}
		return forms["DEFAULT"]
	}
	if p := forms[class]; p != "" {
		return p
	}
	parent := map[string]string{"VB": "VERB", "VBD": "VERB", "VBN": "VERB", "VBP": "VERB", "NN": "NOUN"}[class]
	if p := forms[parent]; p != "" {
		return p
	}
	return forms["DEFAULT"]
}
