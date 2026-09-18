package unified

import (
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/umer-78/diffkit/internal/diff"
)

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func format(a, b string, context int) string {
	return Format("a.txt", "b.txt", diff.Myers(lines(a), lines(b)), context)
}

// -- the format --------------------------------------------------------------

func TestIdenticalFilesProduceNothing(t *testing.T) {
	// Not a header with no hunks: this output is piped into patch, and a file
	// header with nothing under it is a patch that does nothing but looks like
	// one.
	if got := format("a\nb", "a\nb", 3); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestASingleChange(t *testing.T) {
	got := format("a\nb\nc", "a\nB\nc", 3)
	want := "--- a.txt\n+++ b.txt\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n"
	if got != want {
		t.Fatalf("got:\n%swant:\n%s", got, want)
	}
}

func TestContextIsLimited(t *testing.T) {
	got := format("1\n2\n3\n4\n5\n6\n7", "1\n2\n3\nX\n5\n6\n7", 1)
	want := "--- a.txt\n+++ b.txt\n@@ -3,3 +3,3 @@\n 3\n-4\n+X\n 5\n"
	if got != want {
		t.Fatalf("got:\n%swant:\n%s", got, want)
	}
}

func TestZeroContext(t *testing.T) {
	got := format("1\n2\n3", "1\nX\n3", 0)
	want := "--- a.txt\n+++ b.txt\n@@ -2 +2 @@\n-2\n+X\n"
	if got != want {
		t.Fatalf("got:\n%swant:\n%s", got, want)
	}
}

func TestDistantChangesBecomeSeparateHunks(t *testing.T) {
	a := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12"
	b := "1\nX\n3\n4\n5\n6\n7\n8\n9\n10\nY\n12"
	hunks := Hunks(diff.Myers(lines(a), lines(b)), 2)
	if len(hunks) != 2 {
		t.Fatalf("got %d hunks, want 2", len(hunks))
	}
	if hunks[0].AStart != 1 || hunks[1].AStart != 9 {
		t.Fatalf("hunks start at %d and %d", hunks[0].AStart, hunks[1].AStart)
	}
}

func TestNearbyChangesMergeIntoOneHunk(t *testing.T) {
	a := "1\n2\n3\n4\n5"
	b := "1\nX\n3\nY\n5"
	if hunks := Hunks(diff.Myers(lines(a), lines(b)), 2); len(hunks) != 1 {
		t.Fatalf("got %d hunks, want 1", len(hunks))
	}
}

func TestAnEmptyRangeIsNumberedFromTheLineBefore(t *testing.T) {
	// A pure insertion has no lines on the old side, and patch(1) reads the
	// number as the line to insert *after*. Off by one here puts every inserted
	// line in the wrong place.
	if got := span(4, 0); got != "3,0" {
		t.Fatalf("got %q, want %q", got, "3,0")
	}
	if got := span(4, 1); got != "4" {
		t.Fatalf("got %q, want %q", got, "4")
	}
	if got := span(4, 2); got != "4,2" {
		t.Fatalf("got %q, want %q", got, "4,2")
	}
}

func TestInsertionAtTheStart(t *testing.T) {
	got := format("b\nc", "a\nb\nc", 3)
	want := "--- a.txt\n+++ b.txt\n@@ -1,2 +1,3 @@\n+a\n b\n c\n"
	if got != want {
		t.Fatalf("got:\n%swant:\n%s", got, want)
	}
}

func TestDeletionOfEverything(t *testing.T) {
	got := format("a\nb", "", 3)
	want := "--- a.txt\n+++ b.txt\n@@ -1,2 +0,0 @@\n-a\n-b\n"
	if got != want {
		t.Fatalf("got:\n%swant:\n%s", got, want)
	}
}

// -- round trip --------------------------------------------------------------

func TestParseRoundTrip(t *testing.T) {
	cases := []struct{ a, b string }{
		{"a\nb\nc", "a\nB\nc"},
		{"1\n2\n3\n4\n5\n6\n7\n8\n9", "1\n2\nX\n4\n5\n6\n7\n8\nY"},
		{"a", "a\nb\nc"},
		{"a\nb\nc", "a"},
		{"", "a\nb"},
	}

	for _, c := range cases {
		a, b := lines(c.a), lines(c.b)
		patch := Format("a", "b", diff.Myers(a, b), 3)
		if patch == "" {
			continue
		}

		hunks, err := Parse(patch)
		if err != nil {
			t.Fatalf("parse %q: %v", patch, err)
		}
		got, err := ApplyHunks(a, hunks)
		if err != nil {
			t.Fatalf("apply %q: %v", patch, err)
		}
		if strings.Join(got, "\n") != strings.Join(b, "\n") {
			t.Fatalf("%q -> %q gave %q", c.a, c.b, got)
		}
	}
}

func TestParseRoundTripOnRandomInput(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	alphabet := []string{"alpha", "beta", "gamma", "delta", "epsilon"}

	for trial := range 1000 {
		a := make([]string, rng.Intn(14))
		b := make([]string, rng.Intn(14))
		for i := range a {
			a[i] = alphabet[rng.Intn(len(alphabet))]
		}
		for i := range b {
			b[i] = alphabet[rng.Intn(len(alphabet))]
		}

		patch := Format("a", "b", diff.Myers(a, b), rng.Intn(4))
		if patch == "" {
			continue
		}
		hunks, err := Parse(patch)
		if err != nil {
			t.Fatalf("trial %d: parse: %v\n%s", trial, err, patch)
		}
		got, err := ApplyHunks(a, hunks)
		if err != nil {
			t.Fatalf("trial %d: apply: %v\n%s", trial, err, patch)
		}
		if strings.Join(got, "\x00") != strings.Join(b, "\x00") {
			t.Fatalf("trial %d: %q -> %q gave %q\n%s", trial, a, b, got, patch)
		}
	}
}

// -- parse errors ------------------------------------------------------------

func TestParseRejectsContentBeforeAHeader(t *testing.T) {
	if _, err := Parse(" a\n"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestParseRejectsABadHeader(t *testing.T) {
	for _, patch := range []string{"@@ x +1 @@\n", "@@ -1 @@\n", "@@ -a,b +1 @@\n", "@@ -1,z +1 @@\n"} {
		if _, err := Parse(patch); err == nil {
			t.Fatalf("expected an error for %q", patch)
		}
	}
}

func TestParseRejectsAnUnknownLinePrefix(t *testing.T) {
	if _, err := Parse("@@ -1 +1 @@\n?what\n"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestApplyRefusesAPatchThatDoesNotMatch(t *testing.T) {
	// The whole point of the context lines. A patch applied without checking
	// them silently corrupts a file that has moved on since the patch was made.
	hunks, err := Parse("@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyHunks([]string{"a", "DIFFERENT", "c"}, hunks); err == nil {
		t.Fatal("expected an error")
	}
}

func TestApplyRefusesAPatchPastTheEnd(t *testing.T) {
	hunks, _ := Parse("@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n")
	if _, err := ApplyHunks([]string{"a"}, hunks); err == nil {
		t.Fatal("expected an error")
	}
}

func TestApplyLeavesTheRestOfTheFileAlone(t *testing.T) {
	hunks, _ := Parse("@@ -1,2 +1,2 @@\n a\n-b\n+B\n")
	got, err := ApplyHunks([]string{"a", "b", "c", "d"}, hunks)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "a,B,c,d" {
		t.Fatalf("got %q", got)
	}
}

// -- against GNU diff --------------------------------------------------------

func TestOutputMatchesGNUDiff(t *testing.T) {
	// Checking a formatter against its own parser checks nothing. This compares
	// the hunk headers and bodies against the tool everything else in the world
	// reads, which is the only definition of "correct" that matters here.
	path, err := exec.LookPath("diff")
	if err != nil {
		t.Skip("diff(1) not available")
	}

	cases := []struct{ name, a, b string }{
		{"one change", "1\n2\n3\n4\n5", "1\n2\nX\n4\n5"},
		{"two distant changes", "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12",
			"1\nX\n3\n4\n5\n6\n7\n8\n9\n10\nY\n12"},
		{"insertion", "1\n2\n3", "1\n2\nnew\n3"},
		{"deletion", "1\n2\n3", "1\n3"},
		{"append", "1\n2", "1\n2\n3"},
		{"prepend", "2\n3", "1\n2\n3"},
		{"delete everything", "1\n2\n3", ""},
		{"replace everything", "1\n2\n3", "x\ny\nz"},
	}

	dir := t.TempDir()
	for _, c := range cases {
		for _, context := range []int{0, 1, 3} {
			t.Run(c.name, func(t *testing.T) {
				aPath := filepath.Join(dir, "a.txt")
				bPath := filepath.Join(dir, "b.txt")
				write(t, aPath, c.a)
				write(t, bPath, c.b)

				out, _ := exec.Command(path, "-U", strconv.Itoa(context), aPath, bPath).Output()
				want := stripFileHeaders(string(out))
				got := stripFileHeaders(Format(aPath, bPath, diff.Myers(lines(c.a), lines(c.b)), context))

				if got != want {
					t.Fatalf("context %d\ndiffkit:\n%s\nGNU diff:\n%s", context, got, want)
				}
			})
		}
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if content != "" {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// stripFileHeaders drops the --- / +++ lines, which carry timestamps in GNU
// diff's output and nothing in ours. Everything below them is compared.
func stripFileHeaders(patch string) string {
	var kept []string
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
