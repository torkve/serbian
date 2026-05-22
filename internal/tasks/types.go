package tasks

import "encoding/json"

const (
	KindCloze       = "cloze"
	KindConjugation = "conjugation"
	KindCase        = "case"
	KindAspect      = "aspect"
	KindTrRUSR      = "tr_ru_sr"
	KindTrSRRU      = "tr_sr_ru"
	KindVocab       = "vocab"
	KindSpeak       = "speak"
)

type Modality string

const (
	ModalityGrammar     Modality = "grammar"
	ModalityTranslation Modality = "translation"
	ModalitySpeaking    Modality = "speaking"
)

var modalityByKind = map[string]Modality{
	KindCloze:       ModalityGrammar,
	KindConjugation: ModalityGrammar,
	KindCase:        ModalityGrammar,
	KindAspect:      ModalityGrammar,
	KindTrRUSR:      ModalityTranslation,
	KindTrSRRU:      ModalityTranslation,
	KindVocab:       ModalityTranslation,
	KindSpeak:       ModalitySpeaking,
}

func ModalityOf(kind string) Modality { return modalityByKind[kind] }

func KindsByModality(m Modality) []string {
	var out []string
	for k, v := range modalityByKind {
		if v == m {
			out = append(out, k)
		}
	}
	return out
}

var estSeconds = map[string]int{
	KindCloze: 20, KindConjugation: 15, KindCase: 15, KindAspect: 20,
	KindTrRUSR: 30, KindTrSRRU: 30, KindVocab: 10, KindSpeak: 45,
}

func EstSeconds(kind string) int { return estSeconds[kind] }

type Task struct {
	ID         int64           `json:"id"`
	Kind       string          `json:"kind"`
	Difficulty int             `json:"difficulty"`
	Topic      string          `json:"topic,omitempty"`
	Prompt     string          `json:"prompt"`
	Payload    json.RawMessage `json:"payload"`
	EstSec     int             `json:"est_sec"`
}

type Expected struct {
	Answers     []string `json:"answers"`
	Alt         []string `json:"alt,omitempty"`
	MustContain []string `json:"must_contain,omitempty"`
	// Critical: substrings that MUST appear in the user's answer (after
	// normalization). If any is missing, the grade is 0 regardless of
	// overall similarity. Designed for preposition+case complexes and
	// idiomatic syntagmas where omission is the trap.
	Critical []string `json:"critical,omitempty"`
	// Forbidden: substrings that must NOT appear (Russian-style calques,
	// false friends, the "obvious wrong" surface form the task is testing
	// against). If any appears, the grade is 0.
	Forbidden []string `json:"forbidden,omitempty"`
}
