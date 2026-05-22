package tasks

import (
	"strings"
	"unicode"
)

// Normalize collapses whitespace, lowercases, and strips punctuation so a
// user's free-form answer can be compared against canonical references.
// Works for both Serbian and Russian Cyrillic (and Latin if any leaks in).
func Normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevSpace := true
	for _, r := range s {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			prevSpace = false
		case unicode.IsSpace(r):
			if !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// Levenshtein distance over runes (Unicode-correct).
func Levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

// Similarity returns 1 for identical strings down to 0 for fully different.
func Similarity(a, b string) float64 {
	la, lb := len([]rune(a)), len([]rune(b))
	if la == 0 && lb == 0 {
		return 1
	}
	m := la
	if lb > m {
		m = lb
	}
	if m == 0 {
		return 1
	}
	return 1 - float64(Levenshtein(a, b))/float64(m)
}

func min3(a, b, c int) int {
	if a < b {
		b = a
	}
	if c < b {
		return c
	}
	return b
}
