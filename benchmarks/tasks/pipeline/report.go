package pipeline

// Summary is the end-to-end result of the pipeline.
type Summary struct {
	Windows     int     // windows after merging
	TotalErrors int     // deduplicated errors across all windows
	Worst       string  // service with the highest anomaly score
	WorstScore  float64 // its score
}

// Report runs the whole pipeline on a raw log.
func Report(raw string) (Summary, error) {
	evs, err := ParseAll(raw)
	if err != nil {
		return Summary{}, err
	}
	windows := Windowize(evs)
	Dedupe(windows)
	Rate(windows)
	windows = Merge(windows)
	scores := Score(windows)

	var sum Summary
	sum.Windows = len(windows)
	for _, w := range windows {
		sum.TotalErrors += w.Errors
	}
	for svc, s := range scores {
		if s > sum.WorstScore || (s == sum.WorstScore && (sum.Worst == "" || svc < sum.Worst)) {
			sum.Worst, sum.WorstScore = svc, s
		}
	}
	return sum, nil
}
