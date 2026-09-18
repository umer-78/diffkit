package diff

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// algorithms is every implementation, so the properties that must hold for all
// of them are asserted for all of them rather than for whichever was in mind.
var algorithms = map[string]func(a, b []string) []Edit{
	"myers":    Myers,
	"patience": Patience,
	"table":    Table,
}

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func render(edits []Edit) string {
	var sb strings.Builder
	for _, e := range edits {
		sb.WriteString(e.Op.String())
		sb.WriteString(e.Line)
		sb.WriteString("\n")
	}
	return sb.String()
}

// -- the property every algorithm must satisfy --------------------------------

func TestApplyingTheScriptReproducesTheNewFile(t *testing.T) {
	cases := []struct{ name, a, b string }{
		{"identical", "a\nb\nc", "a\nb\nc"},
		{"empty to empty", "", ""},
		{"empty to full", "", "a\nb"},
		{"full to empty", "a\nb", ""},
		{"one line changed", "a\nb\nc", "a\nB\nc"},
		{"prepend", "b\nc", "a\nb\nc"},
		{"append", "a\nb", "a\nb\nc"},
		{"delete middle", "a\nb\nc", "a\nc"},
		{"completely different", "a\nb\nc", "x\ny\nz"},
		{"repeated lines", "a\na\na", "a\na"},
		{"reordered", "a\nb\nc", "c\nb\na"},
		{"duplicated block", "x\ny", "x\ny\nx\ny"},
	}

	for name, compute := range algorithms {
		for _, c := range cases {
			t.Run(name+"/"+c.name, func(t *testing.T) {
				a, b := lines(c.a), lines(c.b)
				got, err := Apply(a, compute(a, b))
				if err != nil {
					t.Fatalf("apply: %v", err)
				}
				if strings.Join(got, "\n") != strings.Join(b, "\n") {
					t.Fatalf("got %q, want %q", got, b)
				}
			})
		}
	}
}

func TestApplyingTheScriptReproducesTheNewFileOnRandomInput(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	alphabet := []string{"", "{", "}", "a", "b", "c", "d"}

	for name, compute := range algorithms {
		t.Run(name, func(t *testing.T) {
			for trial := range 2000 {
				a := make([]string, rng.Intn(10))
				b := make([]string, rng.Intn(10))
				for i := range a {
					a[i] = alphabet[rng.Intn(len(alphabet))]
				}
				for i := range b {
					b[i] = alphabet[rng.Intn(len(alphabet))]
				}

				got, err := Apply(a, compute(a, b))
				if err != nil {
					t.Fatalf("trial %d: apply %q -> %q: %v", trial, a, b, err)
				}
				if strings.Join(got, "\x00") != strings.Join(b, "\x00") {
					t.Fatalf("trial %d: %q -> %q gave %q", trial, a, b, got)
				}
			}
		})
	}
}

func TestIndicesPointAtTheRightLines(t *testing.T) {
	a, b := lines("a\nb\nc"), lines("a\nB\nc")

	for name, compute := range algorithms {
		t.Run(name, func(t *testing.T) {
			for _, e := range compute(a, b) {
				switch e.Op {
				case Equal:
					if a[e.AIndex] != e.Line || b[e.BIndex] != e.Line {
						t.Fatalf("equal edit %+v does not match either file", e)
					}
				case Delete:
					if e.BIndex != -1 || a[e.AIndex] != e.Line {
						t.Fatalf("delete edit %+v is wrong", e)
					}
				case Insert:
					if e.AIndex != -1 || b[e.BIndex] != e.Line {
						t.Fatalf("insert edit %+v is wrong", e)
					}
				}
			}
		})
	}
}

func TestEqualFilesProduceNoEdits(t *testing.T) {
	a := lines("a\nb\nc")
	for name, compute := range algorithms {
		t.Run(name, func(t *testing.T) {
			if d := Distance(compute(a, a)); d != 0 {
				t.Fatalf("distance %d, want 0", d)
			}
		})
	}
}

// -- Myers is minimal, and that is checkable ---------------------------------

func TestMyersIsMinimal(t *testing.T) {
	// The table algorithm computes a longest common subsequence outright, so
	// its distance is the true minimum. Myers must never exceed it.
	rng := rand.New(rand.NewSource(2))
	alphabet := []string{"a", "b", "c", "d"}

	for range 3000 {
		a := make([]string, rng.Intn(9))
		b := make([]string, rng.Intn(9))
		for i := range a {
			a[i] = alphabet[rng.Intn(len(alphabet))]
		}
		for i := range b {
			b[i] = alphabet[rng.Intn(len(alphabet))]
		}
		if got, want := Distance(Myers(a, b)), Distance(Table(a, b)); got != want {
			t.Fatalf("myers gave %d edits for %q -> %q, the minimum is %d", got, a, b, want)
		}
	}
}

func TestPatienceIsNeverShorterThanMyers(t *testing.T) {
	// It cannot be: Myers is minimal. This is the invariant behind the README's
	// measurement that patience is longer on 17.5% of random inputs — a run in
	// which patience came out shorter would mean Myers had a bug.
	rng := rand.New(rand.NewSource(3))
	alphabet := []string{"{", "}", "", "a", "b", "c"}

	longer := 0
	trials := 3000
	for range trials {
		a := make([]string, 4+rng.Intn(8))
		b := make([]string, 4+rng.Intn(8))
		for i := range a {
			a[i] = alphabet[rng.Intn(len(alphabet))]
		}
		for i := range b {
			b[i] = alphabet[rng.Intn(len(alphabet))]
		}
		m, p := Distance(Myers(a, b)), Distance(Patience(a, b))
		if p < m {
			t.Fatalf("patience gave %d edits for %q -> %q, fewer than the minimum %d", p, a, b, m)
		}
		if p > m {
			longer++
		}
	}
	if longer == 0 {
		t.Fatal("patience was never longer, which means this test is not exercising the difference")
	}
	t.Logf("patience was longer on %d of %d random inputs (%.1f%%)",
		longer, trials, 100*float64(longer)/float64(trials))
}

// -- specific shapes ---------------------------------------------------------

func TestMyersPrefersDeleteBeforeInsert(t *testing.T) {
	// A replaced line should read as the old one going and the new one
	// arriving, in that order. The opposite order is equally short and reads
	// backwards in every review tool.
	got := render(Myers(lines("a\nb\nc"), lines("a\nB\nc")))
	want := " a\n-b\n+B\n c\n"
	if got != want {
		t.Fatalf("got:\n%swant:\n%s", got, want)
	}
}

func TestAnInsertionBetweenFunctionsIsNotSplit(t *testing.T) {
	// The case patience diff is famous for. With this tie-breaking, Myers
	// already gets it right, so the README says so rather than repeating the
	// folklore.
	a := lines("void func1() {\n    x += 1;\n}\n\nvoid func2() {\n    x += 2;\n}")
	b := lines("void func1() {\n    x += 1;\n}\n\nvoid functhreehalves() {\n    x += 1.5;\n}\n\nvoid func2() {\n    x += 2;\n}")

	want := " void func1() {\n     x += 1;\n }\n \n" +
		"+void functhreehalves() {\n+    x += 1.5;\n+}\n+\n" +
		" void func2() {\n     x += 2;\n }\n"

	for name, compute := range algorithms {
		t.Run(name, func(t *testing.T) {
			if got := render(compute(a, b)); got != want {
				t.Fatalf("got:\n%swant:\n%s", got, want)
			}
		})
	}
}

func TestPatienceIgnoresLinesThatAreNotUnique(t *testing.T) {
	// Every line here appears more than once on at least one side, so there is
	// no anchor to hang an alignment on and patience falls back to Myers. The
	// fallback matters: recursing with no anchors and no fallback loops.
	a := lines("x\nx\nx")
	b := lines("x\nx")
	if got, want := render(Patience(a, b)), render(Myers(a, b)); got != want {
		t.Fatalf("expected the Myers fallback, got:\n%s", got)
	}
}

func TestTableAllocatesRegardlessOfSimilarity(t *testing.T) {
	a := make([]string, 200)
	for i := range a {
		a[i] = fmt.Sprintf("line %d", i)
	}
	b := append([]string(nil), a...)

	// Identical files, and the table is still full size.
	if cells := TableCells(a, b); cells != 201*201 {
		t.Fatalf("got %d cells, want %d", cells, 201*201)
	}
	if d := Distance(Table(a, b)); d != 0 {
		t.Fatalf("distance %d, want 0", d)
	}
}

// -- Apply rejects scripts that do not belong to the file --------------------

func TestApplyRejectsAScriptForAnotherFile(t *testing.T) {
	edits := Myers(lines("a\nb"), lines("a\nB"))
	if _, err := Apply(lines("x\ny"), edits); err == nil {
		t.Fatal("expected an error applying a script to the wrong file")
	}
}

func TestApplyRejectsAScriptThatRunsPastTheEnd(t *testing.T) {
	edits := Myers(lines("a\nb\nc"), lines("a"))
	if _, err := Apply(lines("a"), edits); err == nil {
		t.Fatal("expected an error")
	}
}

func TestApplyRejectsAScriptThatStopsShort(t *testing.T) {
	edits := Myers(lines("a"), lines("b"))
	if _, err := Apply(lines("a\nextra"), edits); err == nil {
		t.Fatal("expected an error")
	}
}

func TestCount(t *testing.T) {
	s := Count(Myers(lines("a\nb\nc"), lines("a\nB\nc\nd")))
	if s.Equal != 2 || s.Deleted != 1 || s.Inserted != 2 {
		t.Fatalf("got %+v", s)
	}
}

func TestOpString(t *testing.T) {
	if got := fmt.Sprintf("%s%s%s", Equal, Delete, Insert); got != " -+" {
		t.Fatalf("got %q", got)
	}
}

// -- benchmarks --------------------------------------------------------------

func corpus(n int, changes int) ([]string, []string) {
	a := make([]string, n)
	for i := range a {
		a[i] = fmt.Sprintf("line %d: some plausible content %d", i, i*7%1000)
	}
	b := append([]string(nil), a...)
	for i := 0; i < changes && i*(n/max(changes, 1)) < n; i++ {
		at := i * (n / max(changes, 1))
		b[at] = "changed " + b[at]
	}
	return a, b
}

func BenchmarkMyers(b *testing.B) {
	for _, n := range []int{100, 1000, 4000} {
		x, y := corpus(n, 5)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				Myers(x, y)
			}
		})
	}
}

func BenchmarkPatience(b *testing.B) {
	for _, n := range []int{100, 1000, 4000} {
		x, y := corpus(n, 5)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				Patience(x, y)
			}
		})
	}
}

func BenchmarkTable(b *testing.B) {
	for _, n := range []int{100, 1000, 4000} {
		x, y := corpus(n, 5)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				Table(x, y)
			}
		})
	}
}
