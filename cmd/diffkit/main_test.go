package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func invoke(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func file(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if content != "" {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// -- exit codes --------------------------------------------------------------

func TestExitZeroWhenThereIsNoDifference(t *testing.T) {
	// diff(1)'s convention, so this works in a pipeline and as a CI gate.
	dir := t.TempDir()
	a := file(t, dir, "a.txt", "one\ntwo")
	b := file(t, dir, "b.txt", "one\ntwo")

	code, out, _ := invoke(t, "diff", a, b)
	if code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if out != "" {
		t.Fatalf("expected no output, got %q", out)
	}
}

func TestExitOneWhenFilesDiffer(t *testing.T) {
	dir := t.TempDir()
	a := file(t, dir, "a.txt", "one\ntwo")
	b := file(t, dir, "b.txt", "one\nTWO")

	code, out, _ := invoke(t, "diff", a, b)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(out, "-two") || !strings.Contains(out, "+TWO") {
		t.Fatalf("got:\n%s", out)
	}
}

func TestExitTwoOnAMissingFile(t *testing.T) {
	code, _, errOut := invoke(t, "diff", "no-such-file", "also-missing")
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "diffkit:") {
		t.Fatalf("got %q", errOut)
	}
}

func TestExitTwoOnAnUnknownCommand(t *testing.T) {
	code, _, errOut := invoke(t, "frobnicate")
	if code != 2 || !strings.Contains(errOut, "unknown command") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestExitTwoWithNoArguments(t *testing.T) {
	if code, _, _ := invoke(t); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func TestHelp(t *testing.T) {
	code, out, _ := invoke(t, "--help")
	if code != 0 || !strings.Contains(out, "diffkit diff") {
		t.Fatalf("exit %d, out %q", code, out)
	}
}

// -- the commands ------------------------------------------------------------

func TestDiffContextFlag(t *testing.T) {
	dir := t.TempDir()
	a := file(t, dir, "a.txt", "1\n2\n3\n4\n5\n6\n7")
	b := file(t, dir, "b.txt", "1\n2\n3\nX\n5\n6\n7")

	_, wide, _ := invoke(t, "diff", "-c", "3", a, b)
	_, narrow, _ := invoke(t, "diff", "-c", "1", a, b)
	if len(strings.Split(narrow, "\n")) >= len(strings.Split(wide, "\n")) {
		t.Fatalf("less context should mean fewer lines:\n%s\n%s", narrow, wide)
	}
}

func TestDiffAlgorithmFlag(t *testing.T) {
	dir := t.TempDir()
	a := file(t, dir, "a.txt", "1\n2\n3")
	b := file(t, dir, "b.txt", "1\nX\n3")

	for _, name := range []string{"myers", "patience", "table"} {
		code, out, _ := invoke(t, "diff", "-a", name, a, b)
		if code != 1 || !strings.Contains(out, "+X") {
			t.Fatalf("%s: exit %d, out %q", name, code, out)
		}
	}
}

func TestAnUnknownAlgorithmIsRejected(t *testing.T) {
	dir := t.TempDir()
	a := file(t, dir, "a.txt", "1")
	b := file(t, dir, "b.txt", "2")

	code, _, errOut := invoke(t, "diff", "-a", "quantum", a, b)
	if code != 2 || !strings.Contains(errOut, "unknown algorithm") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestStat(t *testing.T) {
	dir := t.TempDir()
	a := file(t, dir, "a.txt", "1\n2\n3")
	b := file(t, dir, "b.txt", "1\nX\n3\n4")

	code, out, _ := invoke(t, "stat", a, b)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(out, "2 unchanged, 2 inserted, 1 deleted") {
		t.Fatalf("got:\n%s", out)
	}
}

func TestStatExitsZeroWhenIdentical(t *testing.T) {
	dir := t.TempDir()
	a := file(t, dir, "a.txt", "1\n2")
	b := file(t, dir, "b.txt", "1\n2")
	if code, _, _ := invoke(t, "stat", a, b); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
}

func TestDiffThenApplyRoundTrips(t *testing.T) {
	// The whole tool in one test: what `diff` emits, `apply` must consume.
	dir := t.TempDir()
	a := file(t, dir, "a.txt", "alpha\nbeta\ngamma\ndelta\nepsilon")
	b := file(t, dir, "b.txt", "alpha\nBETA\ngamma\ndelta\nepsilon\nzeta")

	_, patch, _ := invoke(t, "diff", a, b)
	patchPath := filepath.Join(dir, "p.diff")
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := invoke(t, "apply", a, patchPath)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	want, _ := os.ReadFile(b)
	if out != string(want) {
		t.Fatalf("got:\n%swant:\n%s", out, want)
	}
}

func TestApplyRefusesAPatchForAnotherFile(t *testing.T) {
	dir := t.TempDir()
	a := file(t, dir, "a.txt", "alpha\nbeta\ngamma")
	b := file(t, dir, "b.txt", "alpha\nBETA\ngamma")
	other := file(t, dir, "other.txt", "completely\ndifferent\nlines")

	_, patch, _ := invoke(t, "diff", a, b)
	patchPath := filepath.Join(dir, "p.diff")
	_ = os.WriteFile(patchPath, []byte(patch), 0o600)

	code, _, errOut := invoke(t, "apply", other, patchPath)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "but the patch expects") {
		t.Fatalf("got %q", errOut)
	}
}

func TestMergeCleanExitsZero(t *testing.T) {
	dir := t.TempDir()
	base := file(t, dir, "base.txt", "a\nb\nc")
	ours := file(t, dir, "ours.txt", "A\nb\nc")
	theirs := file(t, dir, "theirs.txt", "a\nb\nC")

	code, out, errOut := invoke(t, "merge", base, ours, theirs)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if out != "A\nb\nC\n" {
		t.Fatalf("got %q", out)
	}
	if !strings.Contains(errOut, "merged cleanly") {
		t.Fatalf("stderr %q", errOut)
	}
}

func TestMergeConflictExitsOneAndStillPrintsTheFile(t *testing.T) {
	// A conflicted merge is still a file the caller wants, with markers in it.
	// Printing nothing and exiting non-zero would make the tool unusable.
	dir := t.TempDir()
	base := file(t, dir, "base.txt", "a\nb\nc")
	ours := file(t, dir, "ours.txt", "a\nX\nc")
	theirs := file(t, dir, "theirs.txt", "a\nY\nc")

	code, out, errOut := invoke(t, "merge", base, ours, theirs)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(out, "<<<<<<<") || !strings.Contains(out, ">>>>>>>") {
		t.Fatalf("got:\n%s", out)
	}
	if !strings.Contains(errOut, "1 conflict") {
		t.Fatalf("stderr %q", errOut)
	}
}

func TestMergeLabels(t *testing.T) {
	dir := t.TempDir()
	base := file(t, dir, "base.txt", "a\nb")
	ours := file(t, dir, "ours.txt", "a\nX")
	theirs := file(t, dir, "theirs.txt", "a\nY")

	_, out, _ := invoke(t, "merge", "-l", "feature,ancestor,main", base, ours, theirs)
	for _, want := range []string{"<<<<<<< feature", "||||||| ancestor", ">>>>>>> main"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestMergeRejectsTheWrongNumberOfLabels(t *testing.T) {
	dir := t.TempDir()
	f := file(t, dir, "f.txt", "a")
	if code, _, _ := invoke(t, "merge", "-l", "one,two", f, f, f); code != 2 {
		t.Fatal("expected exit 2")
	}
}

func TestCompareReportsEveryAlgorithm(t *testing.T) {
	dir := t.TempDir()
	a := file(t, dir, "a.txt", "1\n2\n3\n4")
	b := file(t, dir, "b.txt", "1\nX\n3\n4")

	code, out, _ := invoke(t, "compare", a, b)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, name := range []string{"myers", "patience", "table", "allocated"} {
		if !strings.Contains(out, name) {
			t.Fatalf("missing %q in:\n%s", name, out)
		}
	}
}

// -- reading files -----------------------------------------------------------

func TestATrailingNewlineDoesNotBecomeAnEmptyLine(t *testing.T) {
	// Without trimming it, every file reads as though it ended with a blank
	// line and two identical files still diff as identical — but a file with
	// no trailing newline differs from one with it by a phantom line.
	dir := t.TempDir()
	withNewline := filepath.Join(dir, "a.txt")
	withoutNewline := filepath.Join(dir, "b.txt")
	_ = os.WriteFile(withNewline, []byte("one\ntwo\n"), 0o600)
	_ = os.WriteFile(withoutNewline, []byte("one\ntwo"), 0o600)

	if code, out, _ := invoke(t, "diff", withNewline, withoutNewline); code != 0 {
		t.Fatalf("exit %d, output:\n%s", code, out)
	}
}

func TestAnEmptyFileHasNoLines(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.txt")
	_ = os.WriteFile(empty, nil, 0o600)
	full := file(t, dir, "full.txt", "a\nb")

	code, out, _ := invoke(t, "stat", empty, full)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "0 lines in, 2 lines out") {
		t.Fatalf("got:\n%s", out)
	}
}
