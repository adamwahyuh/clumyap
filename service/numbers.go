package main

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Money and years, which espeak reads badly ("$0.99" -> "dollar zero point nine nine",
// "2026" -> "two thousand twenty six"), are spelled out first, following misaki's rules.
// Plain numbers, decimals, percents, ordinals and times are left to espeak, which reads them well.

var (
	ones = []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten",
		"eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen"}
	tens   = []string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}
	scales = []struct {
		size int64
		name string
	}{{1e12, "trillion"}, {1e9, "billion"}, {1e6, "million"}, {1e3, "thousand"}}

	currencies = map[string][2]string{"$": {"dollar", "cent"}, "£": {"pound", "pence"}, "€": {"euro", "cent"}}

	moneyRe  = regexp.MustCompile(`([$£€])(\d{1,3}(?:,\d{3})+|\d+)(?:\.(\d+))?(?:\s?(thousand|million|billion|trillion)\b)?`)
	decadeRe = regexp.MustCompile(`\b(1[0-9]|20)([0-9]0)s\b`)
	numberRe = regexp.MustCompile(`\d+(?:[.,:/]\d+)*`)
)

// numWords spells a whole number: 1250 -> "one thousand two hundred fifty".
func numWords(n int64) string {
	switch {
	case n < 20:
		return ones[n]
	case n < 100:
		if n%10 == 0 {
			return tens[n/10]
		}
		return tens[n/10] + "-" + ones[n%10]
	case n < 1000:
		if n%100 == 0 {
			return ones[n/100] + " hundred"
		}
		return ones[n/100] + " hundred " + numWords(n%100)
	}
	for _, s := range scales {
		if n >= s.size {
			out := numWords(n/s.size) + " " + s.name
			if n%s.size != 0 {
				out += " " + numWords(n%s.size)
			}
			return out
		}
	}
	return strconv.FormatInt(n, 10)
}

// yearWords reads a year the way it is said: 1999 nineteen ninety-nine, 1905 nineteen oh-five,
// 1900 nineteen hundred, 2005 two thousand five, 2026 twenty twenty-six.
func yearWords(y int) string {
	high, low := y/100, y%100
	switch {
	case y%1000 == 0, high%10 == 0 && low < 10:
		return numWords(int64(y))
	case low == 0:
		return numWords(int64(high)) + " hundred"
	case low < 10:
		return numWords(int64(high)) + " oh-" + ones[low]
	}
	return numWords(int64(high)) + " " + numWords(int64(low))
}

// plural turns the last word of a spoken number into its plural: "nineteen ninety" -> "nineteen nineties".
func plural(words string) string {
	i := strings.LastIndexAny(words, " -") + 1
	last := words[i:]
	switch {
	case strings.HasSuffix(last, "y"):
		last = last[:len(last)-1] + "ies"
	case strings.HasSuffix(last, "x"):
		last += "es"
	default:
		last += "s"
	}
	return words[:i] + last
}

func money(sym, whole, frac, scale string) string {
	unit, sub := currencies[sym][0], currencies[sym][1]
	n, _ := strconv.ParseInt(strings.ReplaceAll(whole, ",", ""), 10, 64)
	if scale != "" { // "$2.5 million" -> "two point five million dollars"
		out := numWords(n)
		if frac != "" {
			var digits []string
			for _, d := range frac {
				digits = append(digits, ones[d-'0'])
			}
			out += " point " + strings.Join(digits, " ")
		}
		return out + " " + scale + " " + unit + "s"
	}
	cents := 0
	if frac != "" {
		frac = (frac + "0")[:2] // "$4.5" is fifty cents
		cents, _ = strconv.Atoi(frac)
	}
	amount := func(k int64, u string) string {
		if k != 1 && u != "pence" {
			u += "s"
		}
		return numWords(k) + " " + u
	}
	switch {
	case cents == 0:
		return amount(n, unit)
	case n == 0:
		return amount(int64(cents), sub)
	}
	return amount(n, unit) + " and " + amount(int64(cents), sub)
}

func normalizeNumbers(text string) string {
	text = moneyRe.ReplaceAllStringFunc(text, func(s string) string {
		m := moneyRe.FindStringSubmatch(s)
		return money(m[1], m[2], m[3], m[4])
	})
	text = decadeRe.ReplaceAllStringFunc(text, func(s string) string {
		y, _ := strconv.Atoi(s[:4])
		return plural(yearWords(y))
	})
	// A bare four-digit number is read as a year, as misaki does; "1,999", "19.99" and "1999%" are not.
	return replaceMatches(numberRe, text, func(s string, before, after rune) string {
		if len(s) != 4 || strings.Trim(s, "0123456789") != "" || s[0] == '0' || unicode.IsLetter(after) || after == '%' || before == '$' || before == '£' || before == '€' {
			return s
		}
		y, _ := strconv.Atoi(s)
		return yearWords(y)
	})
}

// replaceMatches is ReplaceAllStringFunc that also passes the runes around each match.
func replaceMatches(re *regexp.Regexp, text string, fn func(match string, before, after rune) string) string {
	var b strings.Builder
	last := 0
	for _, loc := range re.FindAllStringIndex(text, -1) {
		before, _ := utf8.DecodeLastRuneInString(text[:loc[0]])
		after, _ := utf8.DecodeRuneInString(text[loc[1]:])
		b.WriteString(text[last:loc[0]])
		b.WriteString(fn(text[loc[0]:loc[1]], before, after))
		last = loc[1]
	}
	return b.String() + text[last:]
}
