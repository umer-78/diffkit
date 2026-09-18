package diff

// Table computes an edit script by the textbook dynamic-programming LCS.
//
// It is here to be measured against, not to be used. Filling an (N+1)×(M+1)
// table of ints costs O(NM) time and memory whether the files differ by one
// line or by all of them: two 4,000-line files need 16 million cells, about
// 128 MB, before a single line has been compared for similarity. Myers pays for
// the differences instead of the size. BenchmarkMyersVersusTable and
// TestBothAlgorithmsAgreeOnLength put numbers on both halves of that claim.
func Table(a, b []string) []Edit {
	n, m := len(a), len(b)

	lengths := make([][]int, n+1)
	for i := range lengths {
		lengths[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lengths[i][j] = lengths[i+1][j+1] + 1
			} else if lengths[i+1][j] >= lengths[i][j+1] {
				lengths[i][j] = lengths[i+1][j]
			} else {
				lengths[i][j] = lengths[i][j+1]
			}
		}
	}

	edits := make([]Edit, 0, n+m)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			edits = append(edits, Edit{Op: Equal, Line: a[i], AIndex: i, BIndex: j})
			i++
			j++
		case lengths[i+1][j] >= lengths[i][j+1]:
			edits = append(edits, Edit{Op: Delete, Line: a[i], AIndex: i, BIndex: -1})
			i++
		default:
			edits = append(edits, Edit{Op: Insert, Line: b[j], AIndex: -1, BIndex: j})
			j++
		}
	}
	for ; i < n; i++ {
		edits = append(edits, Edit{Op: Delete, Line: a[i], AIndex: i, BIndex: -1})
	}
	for ; j < m; j++ {
		edits = append(edits, Edit{Op: Insert, Line: b[j], AIndex: -1, BIndex: j})
	}
	return edits
}

// TableCells is the number of cells Table allocates for these inputs, which is
// the cost the benchmark is about.
func TableCells(a, b []string) int {
	return (len(a) + 1) * (len(b) + 1)
}
