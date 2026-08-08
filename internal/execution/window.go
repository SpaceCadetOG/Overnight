package execution

import "time"

const OrderWindowDuration = 5 * time.Minute

// OrderWindow converts the 05:00-05:05 America/Chicago placement window to
// UTC. Using the timezone database preserves the correct UTC hour across DST.
func OrderWindow(sessionDate time.Time, chicago *time.Location) (time.Time, time.Time) {
	start := time.Date(sessionDate.Year(), sessionDate.Month(), sessionDate.Day(), 5, 0, 0, 0, chicago)
	return start.UTC(), start.Add(OrderWindowDuration).UTC()
}

func WithinOrderWindow(now, sessionDate time.Time, chicago *time.Location) bool {
	start, end := OrderWindow(sessionDate, chicago)
	now = now.UTC()
	return !now.Before(start) && now.Before(end)
}
