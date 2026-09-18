# diffkit

Diff, patch and three-way merge for text files, written from scratch in Go with
no dependencies. Myers' O(ND) algorithm, patience diff, unified-diff output that
matches GNU diff byte for byte, a patch reader that refuses to apply a patch to
the wrong file, and a diff3-style merge.

```
$ diffkit diff testdata/config-old.txt testdata/config-new.txt
--- testdata/config-old.txt
+++ testdata/config-new.txt
@@ -1,13 +1,14 @@
 server:
   host: 0.0.0.0
-  port: 8080
+  port: 8443
   timeout: 30s
+  tls: true
 
 database:
   driver: postgres
   host: db.internal
-  pool: 10
+  pool: 25
 
 logging:
-  level: info
+  level: debug
   format: json
```

- **148 tests** passing under `-race`, Go 1.22, no dependencies
- Hunk output is compared against **GNU diff** in the test suite and in CI —
  checking a formatter against its own parser checks nothing
- Exits 0 for no difference, 1 for differences or conflicts, 2 for an error, so
  it works in a pipeline and as a CI gate

## Quick start

```bash
git clone https://github.com/umer-78/diffkit.git
cd diffkit
go test ./...
go build -o diffkit ./cmd/diffkit

./diffkit diff    testdata/config-old.txt testdata/config-new.txt
./diffkit stat    testdata/config-old.txt testdata/config-new.txt
./diffkit merge   testdata/merge-base.txt testdata/merge-ours.txt testdata/merge-theirs.txt
./diffkit compare big-a.txt big-b.txt
```

## The table costs 59× the memory to reach the same answer

The dynamic-programming LCS everyone is taught fills an (N+1)×(M+1) table of
integers. Myers walks diagonals of the edit graph instead, keeping the furthest
point reached for each number of edits D, at O((N+M)D). The difference is not
constant-factor. On two 4,000-line files that differ by 11 edits:

```
$ diffkit compare big_a.txt big_b.txt
big_a.txt: 4000 lines    big_b.txt: 4001 lines

algorithm    distance         time      allocated
myers              11      1.246ms        2176 KB
patience           11      1.204ms        1120 KB
table              11    197.076ms      128448 KB

the table algorithm allocates 16012002 cells regardless of how similar the files are
```

**158× slower and 59× the memory**, for exactly the same eleven edits. The
table's cost is set by the size of the files; Myers' is set by how much they
differ, which on a code change is almost nothing. `internal/diff/lcs.go` keeps
the table version so the comparison can be run rather than asserted, and
`BenchmarkTable` against `BenchmarkMyers` shows the same curve:

| lines | Myers | table |
| --- | --- | --- |
| 100 | 57 µs, 54 KB | 135 µs, 101 KB |
| 1,000 | 271 µs, 524 KB | 9.1 ms, 8.3 MB |
| 4,000 | 962 µs, 2.1 MB | 110 ms, 131 MB |

The table earns its place in the test suite, though: it computes a true longest
common subsequence, so its edit count is the real minimum. `TestMyersIsMinimal`
checks Myers against it on 3,000 random inputs, which is a stronger claim than
"the output looks right".

## Patience diff did not do what it is famous for

The received wisdom is that patience diff produces more readable diffs than
Myers because it only matches lines that are unique in both files, refusing to
pair up the closing braces and blank lines that make a hunk unreadable. The
canonical example is inserting a function between two others. Measured against
this implementation:

```
 void func1() {
     x += 1;
 }
 
+void functhreehalves() {
+    x += 1.5;
+}
+
 void func2() {
     x += 2;
 }
```

All three algorithms produce exactly that. The failure the example exists to
show does not happen here, because Myers' tie-break — prefer moving *down* the
edit graph when two diagonals are equally far — already keeps the inserted block
together. `TestAnInsertionBetweenFunctionsIsNotSplit` asserts the identical
output from all three.

What patience does reliably do is produce a **longer** script. It cannot produce
a shorter one, since Myers is minimal, and over 20,000 random line sequences:

| | |
| --- | --- |
| patience shorter than Myers | **0.0%** — it never is, and a run where it was would mean Myers had a bug |
| patience equal | 82.5% |
| patience longer | **17.5%** |
| mean edit distance | Myers 12.11, patience **12.63** (4.3% longer) |
| worst case seen | **2.67×** the minimum |

On a moved block the two differ and Myers is the clearer of the two: patience
anchors on the unique function signature and then splits the brace, which is the
exact failure it is meant to prevent. Both algorithms are shipped and selectable
with `-a`, because the honest summary is that the choice matters less than the
tie-breaking inside whichever one you pick — and neither handles a moved block
well, because a line diff has no concept of a move.

## A patch that does not fit is refused

Context lines exist to be checked. A patch applied without checking them
silently corrupts a file that has moved on since the patch was written:

```
$ diffkit apply testdata/merge-base.txt config.diff
diffkit: line 1 is "package config", but the patch expects "server:"
```

Every context line and every deleted line is compared against what is actually
in the file before anything is written. `diffkit diff | diffkit apply` round
trips, and the test suite checks that on 1,000 random file pairs at randomly
chosen context widths — a formatter and a parser written together will agree
with each other on any number of hand-picked cases and still both be wrong.

The hunk headers are the fiddly part. An empty range is numbered from the line
*before* it, because that is the line `patch` inserts after; getting it wrong
puts every inserted line one position off, and produces a patch that applies
cleanly and corrupts the file. `TestAnEmptyRangeIsNumberedFromTheLineBefore`
pins all three forms, and `TestOutputMatchesGNUDiff` compares complete output
against `diff -U0`, `-U1` and `-U3` on eight file pairs.

## Merging: agreement is not conflict

Three-way merge diffs each side against the common ancestor, walks both scripts
together, and cuts regions at the base lines neither side touched. Only a region
both sides changed *differently* is a conflict:

Ours changed `Port`, theirs changed `Pool`, and the result has both:

```
$ diffkit merge testdata/merge-base.txt testdata/merge-ours.txt testdata/merge-theirs.txt
package config

const (
	Port    = 8443
	Timeout = 30
	Pool    = 25
)
merged cleanly into 7 lines
```

The case worth naming is the one a naive line-by-line merge gets wrong: both
sides making the *same* edit. Two people cherry-pick the same fix, or a branch
is rebased onto a commit it already contains, and every line of the region
differs from the ancestor on both sides. That is not a conflict, and diffkit
merges it clean.

Conflicts are as narrow as the disagreement, which took a fix to get right. An
earlier version marked the base lines on either side of an insertion as
unstable, on the reasonable theory that a line added next to a kept line changes
that neighbourhood. But a replacement *is* a delete plus an insert, so the rule
poisoned the seam after every replacement and dragged the following innocent
line into the markers:

Base is `a b c`, ours is `a X c`, theirs is `a Y c`. Only `b` is disputed:

```
    what it used to produce          what it produces now
    ───────────────────────          ────────────────────
    a                                a
    <<<<<<< ours                     <<<<<<< ours
    X                                X
    c            ← innocent          ||||||| base
    ||||||| base                     b
    b                                =======
    c            ← innocent          Y
    =======                          >>>>>>> theirs
    Y                                c
    c            ← innocent
    >>>>>>> theirs
```

`TestAConflictIsNoWiderThanTheDisagreement` is what stops it coming back. Three
further property tests hold for random inputs: merging a file with itself never
conflicts, a change on one side only is always taken whole, and swapping the two
sides never turns a clean merge into a conflict or the reverse.

## Commands

```
diffkit diff    [-c N] [-a myers|patience|table] OLD NEW
diffkit stat    [-a ALGORITHM] OLD NEW
diffkit apply   FILE PATCH
diffkit merge   [-l ours,base,theirs] BASE OURS THEIRS
diffkit compare OLD NEW      run every algorithm and report what each costs
```

A trailing newline terminates the last line; it does not begin an empty one.
Without that, every file reads as though it ended with a blank line, and a file
saved without a trailing newline differs from one saved with it by a line that
does not exist. `TestATrailingNewlineDoesNotBecomeAnEmptyLine` covers it.

## Layout

```
cmd/diffkit/       the command line, and its exit codes
internal/diff/     Myers, patience, the table, and the edit script they share
internal/unified/  hunk grouping, unified-diff output, a parser and an applier
internal/merge/    three-way merge and conflict markers
testdata/          small files the CI exercises end to end
```

Every algorithm is checked through `diff.Apply`: a script that does not
reproduce the new file from the old one is not a diff of them, whatever it looks
like. That property is asserted for all three algorithms over 2,000 random
inputs each, which is how the interesting bugs were found.

```
$ go test -race -cover ./...
ok  github.com/umer-78/diffkit/cmd/diffkit        coverage: 86.5% of statements
ok  github.com/umer-78/diffkit/internal/diff      coverage: 98.1% of statements
ok  github.com/umer-78/diffkit/internal/merge     coverage: 100.0% of statements
ok  github.com/umer-78/diffkit/internal/unified   coverage: 96.5% of statements
```

## Docker

```bash
docker build -t diffkit .
docker run --rm -v "$PWD:/work" diffkit diff /work/a.txt /work/b.txt
```

Distroless, non-root, static binary.

## Licence

MIT.
