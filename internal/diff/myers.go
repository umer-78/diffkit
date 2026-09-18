package diff

// Myers computes a shortest edit script between a and b.
//
// The algorithm walks diagonals of the edit graph, keeping for each number of
// edits D the furthest point reached on every diagonal k. It costs O((N+M)D)
// time and stores one V array per D, so on two files that are nearly identical
// — the usual case — it barely moves, while the textbook dynamic-programming
// LCS fills an N×M table regardless of how similar the inputs are. See lcs.go
// for that version and BenchmarkMyersVersusTable for the difference it makes.
func Myers(a, b []string) []Edit {
	n, m := len(a), len(b)

	// Fast paths that also keep the trace small in the common cases.
	if n == 0 && m == 0 {
		return nil
	}
	if n == 0 {
		return allOf(Insert, b, -1)
	}
	if m == 0 {
		return allOf(Delete, a, -1)
	}

	max := n + m
	offset := max
	v := make([]int, 2*max+1)
	trace := make([][]int, 0, max+1)

	for d := 0; d <= max; d++ {
		trace = append(trace, append([]int(nil), v...))

		for k := -d; k <= d; k += 2 {
			var x int
			// Which of the two neighbouring diagonals to extend from. Moving
			// down is an insertion, moving right a deletion; preferring down
			// on a tie is what makes a replaced block read as "delete then
			// insert" rather than the other way round.
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k

			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x

			if x >= n && y >= m {
				return backtrack(trace, a, b, offset)
			}
		}
	}
	panic("unreachable: an edit script of at most n+m edits always exists")
}

func backtrack(trace [][]int, a, b []string, offset int) []Edit {
	x, y := len(a), len(b)
	edits := make([]Edit, 0, len(a)+len(b))

	for d := len(trace) - 1; d >= 0; d-- {
		v := trace[d]
		k := x - y

		var previousK int
		if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
			previousK = k + 1
		} else {
			previousK = k - 1
		}
		previousX := v[offset+previousK]
		previousY := previousX - previousK

		for x > previousX && y > previousY {
			x--
			y--
			edits = append(edits, Edit{Op: Equal, Line: a[x], AIndex: x, BIndex: y})
		}
		if d == 0 {
			break
		}
		if x == previousX {
			y--
			edits = append(edits, Edit{Op: Insert, Line: b[y], AIndex: -1, BIndex: y})
		} else {
			x--
			edits = append(edits, Edit{Op: Delete, Line: a[x], AIndex: x, BIndex: -1})
		}
	}

	reverse(edits)
	return edits
}

func allOf(op Op, lines []string, missing int) []Edit {
	edits := make([]Edit, len(lines))
	for i, line := range lines {
		e := Edit{Op: op, Line: line, AIndex: i, BIndex: i}
		if op == Insert {
			e.AIndex = missing
		} else {
			e.BIndex = missing
		}
		edits[i] = e
	}
	return edits
}

func reverse(edits []Edit) {
	for i, j := 0, len(edits)-1; i < j; i, j = i+1, j-1 {
		edits[i], edits[j] = edits[j], edits[i]
	}
}
