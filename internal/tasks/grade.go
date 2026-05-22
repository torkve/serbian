package tasks

import (
	"encoding/json"
	"strings"
)

type GradeResult struct {
	Grade      int      `json:"grade"`
	GradedBy   string   `json:"graded_by"`
	Correct    bool     `json:"correct"`
	Similarity float64  `json:"similarity"`
	Feedback   string   `json:"feedback,omitempty"`
	Expected   []string `json:"expected,omitempty"`
	Rationale  string   `json:"rationale,omitempty"`
	Ambiguous  bool     `json:"ambiguous,omitempty"`
	// MissingCritical: which expected.critical substrings the user's
	// answer was missing. Populated only when GradedBy == "critical".
	MissingCritical []string `json:"missing_critical,omitempty"`
	// HitForbidden: which expected.forbidden substrings the user typed.
	// Populated only when GradedBy == "forbidden".
	HitForbidden []string `json:"hit_forbidden,omitempty"`
}

// GradeText handles text answers for cloze, conjugation, case, aspect,
// tr_ru_sr, tr_sr_ru and vocab. Speaking is graded separately via stt.
func GradeText(kind string, expectedJSON []byte, userAnswer string) (GradeResult, error) {
	var exp Expected
	if err := json.Unmarshal(expectedJSON, &exp); err != nil {
		return GradeResult{}, err
	}
	ua := Normalize(userAnswer)

	// Exact match (after normalization) against any reference.
	for _, a := range exp.Answers {
		if Normalize(a) == ua {
			return GradeResult{
				Grade: 5, GradedBy: "exact", Correct: true, Similarity: 1.0,
				Expected: exp.Answers,
			}, nil
		}
	}

	// Choice-based kinds: anything that wasn't an exact match is simply
	// wrong. Wrong options are deliberately spelled close to the right one,
	// so fuzzy similarity would falsely reward near-misses.
	if kind == KindCase || kind == KindAspect {
		return GradeResult{
			Grade: 0, GradedBy: "exact", Correct: false, Similarity: 0,
			Expected: exp.Answers,
		}, nil
	}

	// Critical substrings: required-present hard gate. Bypassed by exact
	// match above (a literal match by definition contains every critical
	// piece). Sits before fuzzy so high-similarity-but-missing-preposition
	// answers can't sneak through.
	if len(exp.Critical) > 0 {
		var missing []string
		for _, c := range exp.Critical {
			if !strings.Contains(ua, Normalize(c)) {
				missing = append(missing, c)
			}
		}
		if len(missing) > 0 {
			return GradeResult{
				Grade: 0, GradedBy: "critical", Correct: false, Similarity: 0,
				Expected: exp.Answers, MissingCritical: missing,
			}, nil
		}
	}

	// Forbidden substrings: required-absent hard gate. Catches Russian
	// calques, false friends, and obvious-wrong surface forms.
	if len(exp.Forbidden) > 0 {
		var hit []string
		for _, f := range exp.Forbidden {
			if strings.Contains(ua, Normalize(f)) {
				hit = append(hit, f)
			}
		}
		if len(hit) > 0 {
			return GradeResult{
				Grade: 0, GradedBy: "forbidden", Correct: false, Similarity: 0,
				Expected: exp.Answers, HitForbidden: hit,
			}, nil
		}
	}

	bestSim := 0.0
	for _, a := range exp.Answers {
		if s := Similarity(Normalize(a), ua); s > bestSim {
			bestSim = s
		}
	}

	// Translation kinds: must_contain heuristic before falling back to similarity.
	if kind == KindTrRUSR || kind == KindTrSRRU {
		if len(exp.MustContain) > 0 {
			all := true
			for _, m := range exp.MustContain {
				if !strings.Contains(ua, Normalize(m)) {
					all = false
					break
				}
			}
			if all && bestSim >= 0.5 {
				return GradeResult{
					Grade: 4, GradedBy: "fuzzy", Correct: true, Similarity: bestSim,
					Expected: exp.Answers,
				}, nil
			}
		}
	}

	grade := simToGrade(bestSim)
	res := GradeResult{
		Grade: grade, GradedBy: "fuzzy",
		Correct: grade >= 3, Similarity: bestSim, Expected: exp.Answers,
	}
	// Mark translations as ambiguous in the band where Claude grading should
	// take over (when available).
	if kind == KindTrRUSR || kind == KindTrSRRU {
		if bestSim >= 0.6 && bestSim < 0.9 {
			res.Ambiguous = true
		}
	}
	return res, nil
}

func simToGrade(s float64) int {
	switch {
	case s >= 0.95:
		return 5
	case s >= 0.85:
		return 4
	case s >= 0.70:
		return 3
	case s >= 0.50:
		return 2
	case s >= 0.30:
		return 1
	default:
		return 0
	}
}
