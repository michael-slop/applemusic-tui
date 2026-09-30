package fx

// ptsLit counts the cells an effect draws.
func ptsLit(e Effect, cols, rows int) int {
	n := 0
	for r := range rows {
		for c := range cols {
			if _, _, ok := e.Cell(c, r); ok {
				n++
			}
		}
	}
	return n
}
