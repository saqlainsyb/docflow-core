package utils

// NeedsRebalance returns true when the gap between two neighboring positions
// is too small for reliable float64 midpoint arithmetic.
// Threshold of 0.001 gives ~30 halvings before hitting this limit.
func NeedsRebalance(below, above float64) bool {
	return (above - below) < 0.001
}

// Between returns the midpoint position between two neighbors.
// Insert before first card:  Between(0, firstPos)  e.g. Between(0, 1000) = 500
// Insert after last card:    Between(lastPos, lastPos+1000)
// Insert between two cards:  Between(posA, posB)
func Between(below, above float64) float64 {
	return (below + above) / 2.0
}