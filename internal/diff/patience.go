package diff

// Patience computes an edit script by matching lines that appear exactly once
// in both files, then recursing between those anchors.
//
// Myers finds *a* shortest script, and shortest is not the same as readable. In
// source code the shortest script often matches up lines that merely look alike
// — a closing brace, a blank line, an `else {` — pairing the end of one
// function with the start of another and producing a hunk that no reader can
// follow. Patience refuses to match a line unless it is unique on both sides,
// which throws away exactly the lines that cause that. The result is sometimes
// a longer script and almost always a clearer one; TestPatienceAlignsBraces
// shows the case.
func Patience(a, b []string) []Edit {
	return patience(a, b, 0, len(a), 0, len(b))
}

func patience(a, b []string, aStart, aEnd, bStart, bEnd int) []Edit {
	// Trim the matching head and tail first: it is cheap and it keeps the
	// unique-line search focused on the part that actually differs.
	var head []Edit
	for aStart < aEnd && bStart < bEnd && a[aStart] == b[bStart] {
		head = append(head, Edit{Op: Equal, Line: a[aStart], AIndex: aStart, BIndex: bStart})
		aStart++
		bStart++
	}
	var tail []Edit
	for aEnd > aStart && bEnd > bStart && a[aEnd-1] == b[bEnd-1] {
		aEnd--
		bEnd--
		tail = append(tail, Edit{Op: Equal, Line: a[aEnd], AIndex: aEnd, BIndex: bEnd})
	}
	reverse(tail)

	middle := patienceMiddle(a, b, aStart, aEnd, bStart, bEnd)
	out := make([]Edit, 0, len(head)+len(middle)+len(tail))
	out = append(out, head...)
	out = append(out, middle...)
	out = append(out, tail...)
	return out
}

func patienceMiddle(a, b []string, aStart, aEnd, bStart, bEnd int) []Edit {
	if aStart == aEnd && bStart == bEnd {
		return nil
	}
	if aStart == aEnd {
		return sliceOf(Insert, b, bStart, bEnd)
	}
	if bStart == bEnd {
		return sliceOf(Delete, a, aStart, aEnd)
	}

	anchors := uniqueCommon(a, b, aStart, aEnd, bStart, bEnd)
	if len(anchors) == 0 {
		// Nothing unique to hang the alignment on, so fall back to Myers over
		// this region. Recursing without a fallback would loop forever.
		return shift(Myers(a[aStart:aEnd], b[bStart:bEnd]), aStart, bStart)
	}

	chain := longestIncreasing(anchors)
	out := make([]Edit, 0, (aEnd-aStart)+(bEnd-bStart))
	previousA, previousB := aStart, bStart
	for _, anchor := range chain {
		out = append(out, patience(a, b, previousA, anchor.a, previousB, anchor.b)...)
		out = append(out, Edit{Op: Equal, Line: a[anchor.a], AIndex: anchor.a, BIndex: anchor.b})
		previousA, previousB = anchor.a+1, anchor.b+1
	}
	out = append(out, patience(a, b, previousA, aEnd, previousB, bEnd)...)
	return out
}

type anchor struct{ a, b int }

// uniqueCommon returns the lines appearing exactly once in each range, paired
// and ordered by their position in a.
func uniqueCommon(a, b []string, aStart, aEnd, bStart, bEnd int) []anchor {
	type seen struct {
		count int
		index int
	}
	inA := make(map[string]seen, aEnd-aStart)
	for i := aStart; i < aEnd; i++ {
		s := inA[a[i]]
		s.count++
		s.index = i
		inA[a[i]] = s
	}
	inB := make(map[string]seen, bEnd-bStart)
	for j := bStart; j < bEnd; j++ {
		s := inB[b[j]]
		s.count++
		s.index = j
		inB[b[j]] = s
	}

	var anchors []anchor
	for i := aStart; i < aEnd; i++ {
		if inA[a[i]].count != 1 {
			continue
		}
		if s, ok := inB[a[i]]; ok && s.count == 1 {
			anchors = append(anchors, anchor{a: i, b: s.index})
		}
	}
	return anchors
}

// longestIncreasing keeps the longest subsequence of anchors whose b indices
// increase, because anchors that cross cannot all be honoured in order. This is
// the patience-sorting step the algorithm is named after.
func longestIncreasing(anchors []anchor) []anchor {
	if len(anchors) == 0 {
		return nil
	}

	piles := make([]int, 0, len(anchors)) // index into anchors of each pile's top
	previous := make([]int, len(anchors)) // back-pointer to the previous pile's card
	for i := range previous {
		previous[i] = -1
	}

	for i, current := range anchors {
		low, high := 0, len(piles)
		for low < high {
			mid := (low + high) / 2
			if anchors[piles[mid]].b < current.b {
				low = mid + 1
			} else {
				high = mid
			}
		}
		if low > 0 {
			previous[i] = piles[low-1]
		}
		if low == len(piles) {
			piles = append(piles, i)
		} else {
			piles[low] = i
		}
	}

	chain := make([]anchor, 0, len(piles))
	for i := piles[len(piles)-1]; i >= 0; i = previous[i] {
		chain = append(chain, anchors[i])
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

func sliceOf(op Op, lines []string, start, end int) []Edit {
	edits := make([]Edit, 0, end-start)
	for i := start; i < end; i++ {
		e := Edit{Op: op, Line: lines[i], AIndex: i, BIndex: i}
		if op == Insert {
			e.AIndex = -1
		} else {
			e.BIndex = -1
		}
		edits = append(edits, e)
	}
	return edits
}

// shift renumbers a script computed over sub-slices back into whole-file indices.
func shift(edits []Edit, aStart, bStart int) []Edit {
	for i := range edits {
		if edits[i].AIndex >= 0 {
			edits[i].AIndex += aStart
		}
		if edits[i].BIndex >= 0 {
			edits[i].BIndex += bStart
		}
	}
	return edits
}
