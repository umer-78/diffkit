// Package diff computes edit scripts between two sequences of lines.
package diff

import "fmt"

// Op is what happened to one line.
type Op int

const (
	Equal Op = iota
	Delete
	Insert
)

func (o Op) String() string {
	switch o {
	case Equal:
		return " "
	case Delete:
		return "-"
	case Insert:
		return "+"
	}
	return "?"
}

// Edit is one line of the script. AIndex and BIndex are the line's position in
// the old and new file, or -1 where it does not exist in that file.
//
// Carrying both indices rather than only the text is what lets a hunk be
// numbered later without re-walking the script and counting, and what makes a
// moved line traceable back to where it came from.
type Edit struct {
	Op     Op
	Line   string
	AIndex int
	BIndex int
}

func (e Edit) String() string {
	return fmt.Sprintf("%s%s", e.Op, e.Line)
}

// Stat counts a script.
type Stat struct {
	Equal    int
	Inserted int
	Deleted  int
}

func Count(edits []Edit) Stat {
	var s Stat
	for _, e := range edits {
		switch e.Op {
		case Equal:
			s.Equal++
		case Insert:
			s.Inserted++
		case Delete:
			s.Deleted++
		}
	}
	return s
}

// Distance is the number of edits, which is the D in Myers' O(ND).
func Distance(edits []Edit) int {
	s := Count(edits)
	return s.Inserted + s.Deleted
}

// Apply replays a script against the old file and returns the new one. A script
// that does not reproduce b from a is not a diff of a and b, whatever it looks
// like, so every algorithm here is tested through this.
func Apply(a []string, edits []Edit) ([]string, error) {
	out := make([]string, 0, len(a))
	at := 0
	for _, e := range edits {
		switch e.Op {
		case Equal, Delete:
			if at >= len(a) {
				return nil, fmt.Errorf("script runs past the end of the old file at line %d", at+1)
			}
			if a[at] != e.Line {
				return nil, fmt.Errorf("line %d is %q in the file but %q in the script", at+1, a[at], e.Line)
			}
			if e.Op == Equal {
				out = append(out, e.Line)
			}
			at++
		case Insert:
			out = append(out, e.Line)
		}
	}
	if at != len(a) {
		return nil, fmt.Errorf("script covers %d of the old file's %d lines", at, len(a))
	}
	return out, nil
}
