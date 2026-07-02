package pipeline

import "math"

// Score computes a per-service anomaly score: for each window, the amount by
// which its error rate exceeds BaseRate, times the window length in minutes.
// Scores are rounded to 2 decimals at the end to be comparison-friendly.
const BaseRate = 1.0

func Score(windows []*Window) map[string]float64 {
	scores := map[string]float64{}
	for _, w := range windows {
		excess := w.ErrPerMin - BaseRate
		if excess <= 0 {
			continue
		}
		minutes := float64(w.End-w.Start) / 60.0
		scores[w.Service] += excess * minutes
	}
	for svc, s := range scores {
		scores[svc] = math.Round(s*100) / 100
	}
	return scores
}
