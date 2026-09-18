// Package merge performs a three-way merge: given a common ancestor and two
// versions derived from it, produce one file, marking the places where the two
// sides changed the same thing differently.
package merge

import (
	"fmt"
	"strings"

	"github.com/umer-78/diffkit/internal/diff"
)

// Result is a merged file plus what had to be left to a human.
type Result struct {
	Lines     []string
	Conflicts []Conflict
}

// Conflict is one region both sides changed, and what each made of it.
type Conflict struct {
	Line   int // 1-based line in the merged output where the marker starts
	Base   []string
	Ours   []string
	Theirs []string
}

// Region is one aligned stretch of the three files.
type region struct {
	base, ours, theirs []string
}

// Merge combines ours and theirs over their common ancestor base.
//
// The algorithm diffs each side against the base, walks the two scripts
// together, and classifies every region: unchanged by both, changed by one, or
// changed by both. Only the last is a conflict, and only when the two changes
// differ — two people making the *same* edit is the single most common false
// conflict in naive line-by-line merges, and it is not a conflict at all.
func Merge(base, ours, theirs []string, labels [3]string) Result {
	regions := align(base, ours, theirs)

	var out []string
	var conflicts []Conflict

	for _, r := range regions {
		ourChange := !equal(r.base, r.ours)
		theirChange := !equal(r.base, r.theirs)

		switch {
		case !ourChange && !theirChange:
			out = append(out, r.base...)
		case ourChange && !theirChange:
			out = append(out, r.ours...)
		case !ourChange && theirChange:
			out = append(out, r.theirs...)
		case equal(r.ours, r.theirs):
			// Both sides made the identical change. Agreement is not conflict.
			out = append(out, r.ours...)
		default:
			conflicts = append(conflicts, Conflict{
				Line: len(out) + 1, Base: r.base, Ours: r.ours, Theirs: r.theirs,
			})
			out = append(out, "<<<<<<< "+labels[0])
			out = append(out, r.ours...)
			out = append(out, "||||||| "+labels[1])
			out = append(out, r.base...)
			out = append(out, "=======")
			out = append(out, r.theirs...)
			out = append(out, ">>>>>>> "+labels[2])
		}
	}
	return Result{Lines: out, Conflicts: conflicts}
}

// align walks both diffs against the base at once, cutting at every base line
// that both sides left untouched. Those lines are the only places the three
// files are known to agree, which makes them the only safe seams.
func align(base, ours, theirs []string) []region {
	ourEdits := diff.Myers(base, ours)
	theirEdits := diff.Myers(base, theirs)

	ourMap := changedBase(ourEdits)
	theirMap := changedBase(theirEdits)

	// For each base line kept by a side, where it landed in that side's file.
	ourAt := landing(ourEdits)
	theirAt := landing(theirEdits)

	var regions []region
	baseStart, ourStart, theirStart := 0, 0, 0

	for i := 0; i <= len(base); i++ {
		stable := i < len(base) && !ourMap[i] && !theirMap[i]
		if !stable && i < len(base) {
			continue
		}

		ourEnd, theirEnd := len(ours), len(theirs)
		if i < len(base) {
			ourEnd, theirEnd = ourAt[i], theirAt[i]
		}

		if baseStart < i || ourStart < ourEnd || theirStart < theirEnd {
			regions = append(regions, region{
				base:   base[baseStart:i],
				ours:   ours[ourStart:ourEnd],
				theirs: theirs[theirStart:theirEnd],
			})
		}
		if i < len(base) {
			regions = append(regions, region{
				base:   base[i : i+1],
				ours:   ours[ourEnd : ourEnd+1],
				theirs: theirs[theirEnd : theirEnd+1],
			})
			baseStart, ourStart, theirStart = i+1, ourEnd+1, theirEnd+1
		}
	}
	return regions
}

// changedBase marks the base lines a side did not keep.
//
// Only deletions count. An earlier version also marked the base lines on either
// side of an insertion, on the theory that a line added next to a kept line
// changes that neighbourhood — but a replacement is a delete plus an insert, so
// that rule poisoned the seam *after* every replacement and dragged an innocent
// line into the conflict region. An insertion needs no such marking: it falls
// into the region between the two seams it sits between, where it is correctly
// seen as one side's change.
func changedBase(edits []diff.Edit) map[int]bool {
	changed := make(map[int]bool)
	for _, e := range edits {
		if e.Op == diff.Delete {
			changed[e.AIndex] = true
		}
	}
	return changed
}

// landing maps each kept base line to its index in the derived file.
func landing(edits []diff.Edit) map[int]int {
	at := make(map[int]int)
	for _, e := range edits {
		if e.Op == diff.Equal {
			at[e.AIndex] = e.BIndex
		}
	}
	return at
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// String renders the merged file.
func (r Result) String() string {
	return strings.Join(r.Lines, "\n")
}

// Summary is one line describing the outcome, for a CLI to print.
func (r Result) Summary() string {
	if len(r.Conflicts) == 0 {
		return fmt.Sprintf("merged cleanly into %d lines", len(r.Lines))
	}
	return fmt.Sprintf("%d conflict(s) in %d lines", len(r.Conflicts), len(r.Lines))
}
