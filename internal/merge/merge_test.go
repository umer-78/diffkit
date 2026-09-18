package merge

import (
	"math/rand"
	"strings"
	"testing"
)

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func run(base, ours, theirs string) Result {
	return Merge(lines(base), lines(ours), lines(theirs), [3]string{"ours", "base", "theirs"})
}

func TestCleanMerges(t *testing.T) {
	cases := []struct{ name, base, ours, theirs, want string }{
		{"nobody changed anything", "a\nb\nc", "a\nb\nc", "a\nb\nc", "a\nb\nc"},
		{"only ours changed", "a\nb\nc", "a\nB\nc", "a\nb\nc", "a\nB\nc"},
		{"only theirs changed", "a\nb\nc", "a\nb\nc", "a\nb\nC", "a\nb\nC"},
		{"different lines", "a\nb\nc\nd\ne", "A\nb\nc\nd\ne", "a\nb\nc\nd\nE", "A\nb\nc\nd\nE"},
		{"adjacent lines", "a\nb\nc\nd", "A\nb\nc\nd", "a\nb\nc\nD", "A\nb\nc\nD"},
		{"ours inserts", "a\nb", "a\nnew\nb", "a\nb", "a\nnew\nb"},
		{"theirs appends", "a", "a", "a\nb", "a\nb"},
		{"ours deletes", "a\nb\nc", "a\nc", "a\nb\nc", "a\nc"},
		{"both sides delete different lines", "a\nb\nc\nd", "b\nc\nd", "a\nb\nc", "b\nc"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := run(c.base, c.ours, c.theirs)
			if len(got.Conflicts) != 0 {
				t.Fatalf("unexpected conflict:\n%s", got)
			}
			if got.String() != c.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}

func TestTheSameChangeOnBothSidesIsNotAConflict(t *testing.T) {
	// The commonest false conflict in a naive line-by-line merge: two people
	// cherry-picked the same fix, or rebased the same commit. Agreement is not
	// disagreement.
	got := run("a\nb\nc", "a\nFIXED\nc", "a\nFIXED\nc")
	if len(got.Conflicts) != 0 {
		t.Fatalf("expected a clean merge, got:\n%s", got)
	}
	if got.String() != "a\nFIXED\nc" {
		t.Fatalf("got:\n%s", got)
	}
}

func TestTheSameInsertionOnBothSidesIsNotAConflict(t *testing.T) {
	got := run("a\nb", "a\nX\nb", "a\nX\nb")
	if len(got.Conflicts) != 0 {
		t.Fatalf("expected a clean merge, got:\n%s", got)
	}
}

func TestConflictingChanges(t *testing.T) {
	got := run("a\nb\nc", "a\nX\nc", "a\nY\nc")
	if len(got.Conflicts) != 1 {
		t.Fatalf("got %d conflicts, want 1", len(got.Conflicts))
	}

	want := "a\n<<<<<<< ours\nX\n||||||| base\nb\n=======\nY\n>>>>>>> theirs\nc"
	if got.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestAConflictIsNoWiderThanTheDisagreement(t *testing.T) {
	// An earlier version marked the base lines on either side of an insertion
	// as unstable. Since a replacement is a delete plus an insert, that dragged
	// the following line into every conflict region — here, the innocent "c".
	got := run("a\nb\nc", "a\nX\nc", "a\nY\nc")
	conflict := got.Conflicts[0]

	if strings.Join(conflict.Base, ",") != "b" {
		t.Fatalf("conflict covers base lines %q, want just b", conflict.Base)
	}
	if strings.Join(conflict.Ours, ",") != "X" || strings.Join(conflict.Theirs, ",") != "Y" {
		t.Fatalf("conflict sides are %q and %q", conflict.Ours, conflict.Theirs)
	}
	if !strings.HasSuffix(got.String(), "\nc") {
		t.Fatalf("the unchanged line should be outside the markers:\n%s", got)
	}
}

func TestOneSideDeletesWhatTheOtherEdits(t *testing.T) {
	got := run("a\nb\nc", "a\nc", "a\nB\nc")
	if len(got.Conflicts) != 1 {
		t.Fatalf("got %d conflicts, want 1", len(got.Conflicts))
	}
	if len(got.Conflicts[0].Ours) != 0 {
		t.Fatalf("our side should be empty, got %q", got.Conflicts[0].Ours)
	}
}

func TestDifferentInsertionsAtTheSamePlaceConflict(t *testing.T) {
	got := run("a\nb", "a\nX\nb", "a\nY\nb")
	if len(got.Conflicts) != 1 {
		t.Fatalf("got %d conflicts, want 1:\n%s", len(got.Conflicts), got)
	}
	if len(got.Conflicts[0].Base) != 0 {
		t.Fatalf("nothing was in the base here, got %q", got.Conflicts[0].Base)
	}
}

func TestConflictLineNumbersPointAtTheMarker(t *testing.T) {
	got := run("a\nb\nc", "a\nX\nc", "a\nY\nc")
	at := got.Conflicts[0].Line
	if got.Lines[at-1] != "<<<<<<< ours" {
		t.Fatalf("line %d is %q, want the opening marker", at, got.Lines[at-1])
	}
}

func TestLabelsAppearInTheMarkers(t *testing.T) {
	got := Merge(lines("a\nb"), lines("a\nX"), lines("a\nY"),
		[3]string{"feature", "main@abc123", "origin/main"})
	text := got.String()
	for _, label := range []string{"<<<<<<< feature", "||||||| main@abc123", ">>>>>>> origin/main"} {
		if !strings.Contains(text, label) {
			t.Fatalf("missing %q in:\n%s", label, text)
		}
	}
}

func TestSummary(t *testing.T) {
	if got := run("a", "a", "a").Summary(); !strings.Contains(got, "cleanly") {
		t.Fatalf("got %q", got)
	}
	if got := run("a\nb\nc", "a\nX\nc", "a\nY\nc").Summary(); !strings.Contains(got, "1 conflict") {
		t.Fatalf("got %q", got)
	}
}

func TestEmptyFiles(t *testing.T) {
	if got := run("", "", ""); len(got.Lines) != 0 || len(got.Conflicts) != 0 {
		t.Fatalf("got %+v", got)
	}
	if got := run("", "a", ""); got.String() != "a" {
		t.Fatalf("got %q", got.String())
	}
}

// -- properties --------------------------------------------------------------

func TestAMergeWithItselfChangesNothing(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	alphabet := []string{"a", "b", "c", "d", "e"}

	for range 500 {
		base := make([]string, rng.Intn(10))
		for i := range base {
			base[i] = alphabet[rng.Intn(len(alphabet))]
		}
		got := Merge(base, base, base, [3]string{"o", "b", "t"})
		if len(got.Conflicts) != 0 {
			t.Fatalf("conflict merging %q with itself:\n%s", base, got)
		}
		if strings.Join(got.Lines, "\x00") != strings.Join(base, "\x00") {
			t.Fatalf("got %q, want %q", got.Lines, base)
		}
	}
}

func TestAOneSidedChangeIsAlwaysTakenWhole(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	alphabet := []string{"a", "b", "c", "d", "e"}

	for trial := range 500 {
		base := make([]string, 1+rng.Intn(8))
		ours := make([]string, 1+rng.Intn(8))
		for i := range base {
			base[i] = alphabet[rng.Intn(len(alphabet))]
		}
		for i := range ours {
			ours[i] = alphabet[rng.Intn(len(alphabet))]
		}

		// theirs is unchanged from base, so the result must be exactly ours.
		got := Merge(base, ours, base, [3]string{"o", "b", "t"})
		if len(got.Conflicts) != 0 {
			t.Fatalf("trial %d: conflict with an unchanged side:\n%s", trial, got)
		}
		if strings.Join(got.Lines, "\x00") != strings.Join(ours, "\x00") {
			t.Fatalf("trial %d: base %q ours %q gave %q", trial, base, ours, got.Lines)
		}
	}
}

func TestMergeIsSymmetricInItsSides(t *testing.T) {
	// Swapping ours and theirs must give the same clean result, or the same
	// conflicts with the sides swapped — never a conflict one way and a clean
	// merge the other.
	rng := rand.New(rand.NewSource(8))
	alphabet := []string{"a", "b", "c", "d"}

	for trial := range 500 {
		base := make([]string, 1+rng.Intn(6))
		ours := make([]string, 1+rng.Intn(6))
		theirs := make([]string, 1+rng.Intn(6))
		for _, s := range [][]string{base, ours, theirs} {
			for i := range s {
				s[i] = alphabet[rng.Intn(len(alphabet))]
			}
		}

		forward := Merge(base, ours, theirs, [3]string{"o", "b", "t"})
		backward := Merge(base, theirs, ours, [3]string{"o", "b", "t"})
		if len(forward.Conflicts) != len(backward.Conflicts) {
			t.Fatalf("trial %d: %d conflicts one way, %d the other\nbase %q ours %q theirs %q",
				trial, len(forward.Conflicts), len(backward.Conflicts), base, ours, theirs)
		}
		if len(forward.Conflicts) == 0 && strings.Join(forward.Lines, "\x00") != strings.Join(backward.Lines, "\x00") {
			t.Fatalf("trial %d: clean merges differ\n%q\n%q", trial, forward.Lines, backward.Lines)
		}
	}
}
