package pipeline

// Rate fills ErrPerMin for every window from its deduplicated error count.
func Rate(windows []*Window) {
	for _, w := range windows {
		minutes := float64(w.End-w.Start) / 60.0
		if minutes <= 0 {
			w.ErrPerMin = 0
			continue
		}
		w.ErrPerMin = float64(w.Errors) / minutes
	}
}
