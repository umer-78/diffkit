// Package unified formats and parses unified diffs — the format patch(1) reads
// and every code review tool displays.
package unified

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/umer-78/diffkit/internal/diff"
)

// Hunk is one run of changes plus the context around it.
type Hunk struct {
	AStart int // 1-based line number in the old file
	ACount int
	BStart int
	BCount int
	Edits  []diff.Edit
}

// Header renders the @@ line. The counts are of lines in each file, so a hunk
// that only inserts still spans the context lines on the old side.
func (h Hunk) Header() string {
	return fmt.Sprintf("@@ -%s +%s @@", span(h.AStart, h.ACount), span(h.BStart, h.BCount))
}

func span(start, count int) string {
	if count == 1 {
		return strconv.Itoa(start)
	}
	if count == 0 {
		// An empty range is numbered from the line *before* it, which is how
		// patch knows where to insert. Getting this wrong puts every inserted
		// line one position off.
		return fmt.Sprintf("%d,0", start-1)
	}
	return fmt.Sprintf("%d,%d", start, count)
}

// Hunks groups an edit script into hunks with at most `context` unchanged lines
// on each side, merging hunks that would otherwise overlap.
func Hunks(edits []diff.Edit, context int) []Hunk {
	if context < 0 {
		context = 0
	}

	changed := make([]int, 0, len(edits))
	for i, e := range edits {
		if e.Op != diff.Equal {
			changed = append(changed, i)
		}
	}
	if len(changed) == 0 {
		return nil
	}

	var hunks []Hunk
	start := max(0, changed[0]-context)
	end := min(len(edits), changed[0]+context+1)

	for _, i := range changed[1:] {
		if i-context <= end {
			end = min(len(edits), i+context+1)
			continue
		}
		hunks = append(hunks, build(edits, start, end))
		start = max(0, i-context)
		end = min(len(edits), i+context+1)
	}
	hunks = append(hunks, build(edits, start, end))
	return hunks
}

func build(edits []diff.Edit, start, end int) Hunk {
	h := Hunk{Edits: edits[start:end]}

	// Line numbers come from the indices carried on the edits rather than from
	// counting, so a hunk is numbered correctly however it was sliced.
	h.AStart, h.BStart = -1, -1
	for _, e := range h.Edits {
		if e.AIndex >= 0 && h.AStart < 0 {
			h.AStart = e.AIndex + 1
		}
		if e.BIndex >= 0 && h.BStart < 0 {
			h.BStart = e.BIndex + 1
		}
		if e.Op != diff.Insert {
			h.ACount++
		}
		if e.Op != diff.Delete {
			h.BCount++
		}
	}
	if h.AStart < 0 {
		h.AStart = firstAfter(edits, start, true)
	}
	if h.BStart < 0 {
		h.BStart = firstAfter(edits, start, false)
	}
	return h
}

// firstAfter finds the line number a pure-insert (or pure-delete) hunk sits at
// on the side it does not touch.
func firstAfter(edits []diff.Edit, start int, side bool) int {
	for i := start - 1; i >= 0; i-- {
		if side && edits[i].AIndex >= 0 {
			return edits[i].AIndex + 2
		}
		if !side && edits[i].BIndex >= 0 {
			return edits[i].BIndex + 2
		}
	}
	return 1
}

// Format renders a complete unified diff. An empty script gives empty output,
// not a header with no hunks: tools pipe this into `patch`, and a file header
// with nothing under it is a patch that does nothing but still looks like one.
func Format(aName, bName string, edits []diff.Edit, context int) string {
	hunks := Hunks(edits, context)
	if len(hunks) == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", aName, bName)
	for _, h := range hunks {
		sb.WriteString(h.Header())
		sb.WriteString("\n")
		for _, e := range h.Edits {
			sb.WriteString(e.Op.String())
			sb.WriteString(e.Line)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// Parse reads a unified diff back into hunks. Round-tripping is the only real
// test that the format is correct: a formatter checked against its own reader
// is checked against nothing, so the tests also apply parsed hunks to the old
// file and compare with the new one.
func Parse(patch string) ([]Hunk, error) {
	var hunks []Hunk
	var current *Hunk

	scanner := bufio.NewScanner(strings.NewReader(patch))
	line := 0
	for scanner.Scan() {
		line++
		text := scanner.Text()

		switch {
		case strings.HasPrefix(text, "--- "), strings.HasPrefix(text, "+++ "):
			continue
		case strings.HasPrefix(text, "@@"):
			if current != nil {
				hunks = append(hunks, *current)
			}
			h, err := parseHeader(text)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			current = &h
		case current == nil:
			return nil, fmt.Errorf("line %d: content before any @@ header", line)
		case text == "":
			// A context line that is an empty line loses its leading space in
			// transit through many tools; treat it as context rather than
			// refusing the patch.
			current.Edits = append(current.Edits, diff.Edit{Op: diff.Equal, AIndex: -1, BIndex: -1})
		default:
			op, ok := opOf(text[0])
			if !ok {
				return nil, fmt.Errorf("line %d: %q does not start with ' ', '-' or '+'", line, text)
			}
			current.Edits = append(current.Edits, diff.Edit{Op: op, Line: text[1:], AIndex: -1, BIndex: -1})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if current != nil {
		hunks = append(hunks, *current)
	}
	return hunks, nil
}

func opOf(c byte) (diff.Op, bool) {
	switch c {
	case ' ':
		return diff.Equal, true
	case '-':
		return diff.Delete, true
	case '+':
		return diff.Insert, true
	}
	return 0, false
}

func parseHeader(text string) (Hunk, error) {
	var h Hunk
	fields := strings.Fields(text)
	if len(fields) < 3 || fields[0] != "@@" {
		return h, fmt.Errorf("malformed hunk header %q", text)
	}

	a, err := parseSpan(fields[1], '-')
	if err != nil {
		return h, err
	}
	b, err := parseSpan(fields[2], '+')
	if err != nil {
		return h, err
	}
	h.AStart, h.ACount = a[0], a[1]
	h.BStart, h.BCount = b[0], b[1]
	return h, nil
}

func parseSpan(field string, sign byte) ([2]int, error) {
	var out [2]int
	if len(field) < 2 || field[0] != sign {
		return out, fmt.Errorf("expected a %c range, got %q", sign, field)
	}
	body := field[1:]
	start, count, found := strings.Cut(body, ",")

	value, err := strconv.Atoi(start)
	if err != nil {
		return out, fmt.Errorf("bad line number in %q", field)
	}
	out[0] = value
	out[1] = 1
	if found {
		value, err = strconv.Atoi(count)
		if err != nil {
			return out, fmt.Errorf("bad line count in %q", field)
		}
		out[1] = value
	}
	return out, nil
}

// ApplyHunks replays parsed hunks against a file. It verifies every context and
// deleted line against what is actually there — a patch applied without
// checking is a patch that silently corrupts a file that has moved on.
func ApplyHunks(a []string, hunks []Hunk) ([]string, error) {
	out := make([]string, 0, len(a))
	at := 0

	for _, h := range hunks {
		target := h.AStart - 1
		if h.ACount == 0 {
			target = h.AStart
		}
		if target < at || target > len(a) {
			return nil, fmt.Errorf("hunk at line %d cannot be applied here", h.AStart)
		}
		out = append(out, a[at:target]...)
		at = target

		for _, e := range h.Edits {
			switch e.Op {
			case diff.Equal, diff.Delete:
				if at >= len(a) {
					return nil, fmt.Errorf("hunk at line %d runs past the end of the file", h.AStart)
				}
				if a[at] != e.Line {
					return nil, fmt.Errorf("line %d is %q, but the patch expects %q", at+1, a[at], e.Line)
				}
				if e.Op == diff.Equal {
					out = append(out, a[at])
				}
				at++
			case diff.Insert:
				out = append(out, e.Line)
			}
		}
	}
	return append(out, a[at:]...), nil
}
