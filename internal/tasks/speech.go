package tasks

import (
	"encoding/json"
	"strings"
	"unicode"
)

// NormalizeCyrillicOnly strips everything that isn't a Cyrillic letter,
// digit or whitespace. Used for comparing Whisper transcripts to target
// sentences — Whisper may mistranscribe parts as Latin, but the user is
// trying to speak Serbian; we want to compare only the Cyrillic content.
func NormalizeCyrillicOnly(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevSpace := true
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			b.WriteRune(r)
			prevSpace = false
		case unicode.IsDigit(r):
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

// GradeSpeech compares a Whisper transcript against the speak task's expected
// answers using Cyrillic-only normalized Levenshtein similarity.
func GradeSpeech(expectedJSON []byte, transcript string) (GradeResult, error) {
	var exp Expected
	if err := json.Unmarshal(expectedJSON, &exp); err != nil {
		return GradeResult{}, err
	}
	norm := NormalizeCyrillicOnly(transcript)

	bestSim := 0.0
	for _, a := range exp.Answers {
		normExpected := NormalizeCyrillicOnly(a)
		if normExpected == norm && norm != "" {
			return GradeResult{
				Grade:      5,
				GradedBy:   "speech-exact",
				Correct:    true,
				Similarity: 1.0,
				Expected:   exp.Answers,
				Feedback:   "Транскрипт: " + transcript,
			}, nil
		}
		if s := Similarity(normExpected, norm); s > bestSim {
			bestSim = s
		}
	}
	grade := simToGrade(bestSim)
	return GradeResult{
		Grade:      grade,
		GradedBy:   "speech-fuzzy",
		Correct:    grade >= 3,
		Similarity: bestSim,
		Expected:   exp.Answers,
		Feedback:   "Транскрипт: " + transcript,
	}, nil
}
