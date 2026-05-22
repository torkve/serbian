package tasks

import "time"

// SRSState mirrors the srs_state row.
type SRSState struct {
	EF           float64
	IntervalDays float64
	Reps         int
	Lapses       int
	DueAt        time.Time
	LastGrade    int
	LastSeenAt   time.Time
}

// UpdateSRS applies the classic SM-2 update for an SRS state given a grade
// (0..5) and the current time. Returns the new state.
func UpdateSRS(s SRSState, grade int, now time.Time) SRSState {
	if grade < 0 {
		grade = 0
	}
	if grade > 5 {
		grade = 5
	}
	out := s
	if out.EF == 0 {
		out.EF = 2.5
	}
	out.LastGrade = grade
	out.LastSeenAt = now

	if grade >= 3 {
		switch out.Reps {
		case 0:
			out.IntervalDays = 1
		case 1:
			out.IntervalDays = 6
		default:
			out.IntervalDays = out.IntervalDays * out.EF
		}
		out.Reps++
	} else {
		out.Reps = 0
		out.IntervalDays = 1
		out.Lapses++
	}

	g := float64(grade)
	out.EF = s.EF + (0.1 - (5-g)*(0.08+(5-g)*0.02))
	if out.EF < 1.3 {
		out.EF = 1.3
	}

	out.DueAt = now.Add(time.Duration(out.IntervalDays * float64(24*time.Hour)))
	return out
}
