// Command diffkit diffs, patches and merges text files.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/umer-78/diffkit/internal/diff"
	"github.com/umer-78/diffkit/internal/merge"
	"github.com/umer-78/diffkit/internal/unified"
)

const usage = `diffkit — diff, patch and merge text files

  diffkit diff    [-c N] [-a myers|patience|table] OLD NEW
  diffkit stat    [-a ALGORITHM] OLD NEW
  diffkit apply   FILE PATCH
  diffkit merge   [-l ours,base,theirs] BASE OURS THEIRS
  diffkit compare OLD NEW      run every algorithm and report what each costs

Exit status: 0 no differences or a clean merge, 1 differences or conflicts,
2 something went wrong. This is what diff(1) does, so diffkit works in a
pipeline and as a CI gate.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(errOut, usage)
		return 2
	}

	var err error
	var code int
	switch args[0] {
	case "diff":
		code, err = cmdDiff(args[1:], out)
	case "stat":
		code, err = cmdStat(args[1:], out)
	case "apply":
		code, err = cmdApply(args[1:], out)
	case "merge":
		code, err = cmdMerge(args[1:], out, errOut)
	case "compare":
		code, err = cmdCompare(args[1:], out)
	case "-h", "--help", "help":
		fmt.Fprint(out, usage)
		return 0
	default:
		fmt.Fprintf(errOut, "diffkit: unknown command %q\n\n%s", args[0], usage)
		return 2
	}

	if err != nil {
		fmt.Fprintf(errOut, "diffkit: %v\n", err)
		return 2
	}
	return code
}

func algorithm(name string) (func(a, b []string) []diff.Edit, error) {
	switch name {
	case "myers":
		return diff.Myers, nil
	case "patience":
		return diff.Patience, nil
	case "table":
		return diff.Table, nil
	}
	return nil, fmt.Errorf("unknown algorithm %q (myers, patience or table)", name)
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(data)
	// A trailing newline terminates the last line; it does not begin an empty
	// one. Without this every file diffs as though it ended with a blank line.
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

func cmdDiff(args []string, out io.Writer) (int, error) {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(out)
	context := fs.Int("c", 3, "lines of context around each change")
	name := fs.String("a", "myers", "algorithm: myers, patience or table")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	if fs.NArg() != 2 {
		return 2, errors.New("diff needs exactly two files")
	}

	compute, err := algorithm(*name)
	if err != nil {
		return 2, err
	}
	a, err := readLines(fs.Arg(0))
	if err != nil {
		return 2, err
	}
	b, err := readLines(fs.Arg(1))
	if err != nil {
		return 2, err
	}

	patch := unified.Format(fs.Arg(0), fs.Arg(1), compute(a, b), *context)
	if patch == "" {
		return 0, nil
	}
	fmt.Fprint(out, patch)
	return 1, nil
}

func cmdStat(args []string, out io.Writer) (int, error) {
	fs := flag.NewFlagSet("stat", flag.ContinueOnError)
	fs.SetOutput(out)
	name := fs.String("a", "myers", "algorithm: myers, patience or table")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	if fs.NArg() != 2 {
		return 2, errors.New("stat needs exactly two files")
	}

	compute, err := algorithm(*name)
	if err != nil {
		return 2, err
	}
	a, err := readLines(fs.Arg(0))
	if err != nil {
		return 2, err
	}
	b, err := readLines(fs.Arg(1))
	if err != nil {
		return 2, err
	}

	edits := compute(a, b)
	s := diff.Count(edits)
	fmt.Fprintf(out, "%s -> %s\n", fs.Arg(0), fs.Arg(1))
	fmt.Fprintf(out, "  %d lines in, %d lines out\n", len(a), len(b))
	fmt.Fprintf(out, "  %d unchanged, %d inserted, %d deleted (edit distance %d)\n",
		s.Equal, s.Inserted, s.Deleted, diff.Distance(edits))
	if s.Inserted+s.Deleted == 0 {
		return 0, nil
	}
	return 1, nil
}

func cmdApply(args []string, out io.Writer) (int, error) {
	if len(args) != 2 {
		return 2, errors.New("apply needs a file and a patch")
	}
	a, err := readLines(args[0])
	if err != nil {
		return 2, err
	}
	data, err := os.ReadFile(args[1])
	if err != nil {
		return 2, err
	}

	hunks, err := unified.Parse(string(data))
	if err != nil {
		return 2, err
	}
	result, err := unified.ApplyHunks(a, hunks)
	if err != nil {
		return 2, err
	}
	for _, line := range result {
		fmt.Fprintln(out, line)
	}
	return 0, nil
}

func cmdMerge(args []string, out, errOut io.Writer) (int, error) {
	fs := flag.NewFlagSet("merge", flag.ContinueOnError)
	fs.SetOutput(out)
	labels := fs.String("l", "ours,base,theirs", "conflict marker labels")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	if fs.NArg() != 3 {
		return 2, errors.New("merge needs a base, ours and theirs")
	}

	parts := strings.Split(*labels, ",")
	if len(parts) != 3 {
		return 2, errors.New("-l takes three comma-separated labels")
	}

	files := make([][]string, 3)
	for i := range files {
		lines, err := readLines(fs.Arg(i))
		if err != nil {
			return 2, err
		}
		files[i] = lines
	}

	result := merge.Merge(files[0], files[1], files[2], [3]string{parts[0], parts[1], parts[2]})
	for _, line := range result.Lines {
		fmt.Fprintln(out, line)
	}
	fmt.Fprintf(errOut, "%s\n", result.Summary())
	if len(result.Conflicts) > 0 {
		return 1, nil
	}
	return 0, nil
}

func cmdCompare(args []string, out io.Writer) (int, error) {
	if len(args) != 2 {
		return 2, errors.New("compare needs exactly two files")
	}
	a, err := readLines(args[0])
	if err != nil {
		return 2, err
	}
	b, err := readLines(args[1])
	if err != nil {
		return 2, err
	}

	fmt.Fprintf(out, "%s: %d lines    %s: %d lines\n\n", args[0], len(a), args[1], len(b))
	fmt.Fprintf(out, "%-10s %10s %12s %14s\n", "algorithm", "distance", "time", "allocated")

	for _, name := range []string{"myers", "patience", "table"} {
		compute, _ := algorithm(name)

		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		start := time.Now()
		edits := compute(a, b)
		elapsed := time.Since(start)
		runtime.ReadMemStats(&after)

		fmt.Fprintf(out, "%-10s %10d %12s %11d KB\n", name, diff.Distance(edits),
			elapsed.Round(time.Microsecond), (after.TotalAlloc-before.TotalAlloc)/1024)
	}

	fmt.Fprintf(out, "\nthe table algorithm allocates %d cells regardless of how similar the files are\n",
		diff.TableCells(a, b))
	return 0, nil
}
