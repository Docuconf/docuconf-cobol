package gen

import (
	"fmt"
	"regexp/syntax"
	"slices"
	"strings"
	"unicode"
)

// A pattern is checked by the loader itself: generate compiles the RE2
// pattern with Go's regexp/syntax, the same compiler docuconf exec uses,
// and writes the program into the loader as two tables. The runtime
// paragraph DC-RX-MATCH runs it as a Pike VM over the value's code
// points, so a value matches in the loader exactly when it matches in Go
// (anywhere in the value, as SPEC §4.3 says).
const (
	rxMaxInsts  = 1000 // DC-RX-INST OCCURS
	rxMaxRanges = 4000 // DC-RX-RANGE OCCURS
	rxInstWidth = 19   // op, out, arg, first range, range count
	rxRangeW    = 14   // lo, hi
)

// rxProg is a compiled pattern as the loader's tables hold it.
type rxProg struct {
	start  int // 1-based
	insts  string
	ranges string
	n      int // instructions
}

// compileRE2 compiles pattern for the loader. It returns an error when
// the pattern is not RE2, and nil (with no error) when its program does
// not fit the loader's tables; docuconf exec then checks it alone.
func compileRE2(pattern string) (*rxProg, error) {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, err
	}
	prog, err := syntax.Compile(re.Simplify())
	if err != nil {
		return nil, err
	}
	if len(prog.Inst) > rxMaxInsts {
		return nil, nil
	}
	var insts, ranges strings.Builder
	nr := 0
	for _, in := range prog.Inst {
		op, out, arg := "F", int(in.Out)+1, 0
		first, count := 0, 0
		switch in.Op {
		case syntax.InstAlt, syntax.InstAltMatch:
			op, arg = "S", int(in.Arg)+1
		case syntax.InstCapture, syntax.InstNop:
			op = "J"
		case syntax.InstEmptyWidth:
			op, arg = "E", int(in.Arg)
		case syntax.InstMatch:
			op = "M"
		case syntax.InstFail:
			op = "F"
		case syntax.InstRuneAny:
			op = "A"
		case syntax.InstRuneAnyNotNL:
			op = "N"
		case syntax.InstRune, syntax.InstRune1:
			op = "R"
			rs := runeRanges(in.Rune, syntax.Flags(in.Arg)&syntax.FoldCase != 0)
			first, count = nr+1, len(rs)
			for _, r := range rs {
				fmt.Fprintf(&ranges, "%07d%07d", r[0], r[1])
			}
			nr += len(rs)
		}
		if nr > rxMaxRanges {
			return nil, nil
		}
		fmt.Fprintf(&insts, "%s%04d%04d%05d%05d", op, out, arg, first, count)
	}
	return &rxProg{start: prog.Start + 1, insts: insts.String(), ranges: ranges.String(), n: len(prog.Inst)}, nil
}

// runeRanges turns an InstRune's rune list (pairs of lo, hi; a single
// rune for InstRune1) into ranges, adding every case-folded equivalent
// when the instruction folds case.
func runeRanges(runes []rune, fold bool) [][2]rune {
	var rs [][2]rune
	if len(runes) == 1 {
		rs = append(rs, [2]rune{runes[0], runes[0]})
	} else {
		for i := 0; i+1 < len(runes); i += 2 {
			rs = append(rs, [2]rune{runes[i], runes[i+1]})
		}
	}
	if !fold {
		return rs
	}
	var extra [][2]rune
	for _, r := range rs {
		for c := r[0]; c <= r[1]; c++ {
			for f := unicode.SimpleFold(c); f != c; f = unicode.SimpleFold(f) {
				extra = append(extra, [2]rune{f, f})
			}
		}
	}
	return mergeRanges(append(rs, extra...))
}

func mergeRanges(rs [][2]rune) [][2]rune {
	slices.SortFunc(rs, func(a, b [2]rune) int { return int(a[0] - b[0]) })
	var out [][2]rune
	for _, r := range rs {
		if n := len(out); n > 0 && r[0] <= out[n-1][1]+1 {
			out[n-1][1] = max(out[n-1][1], r[1])
			continue
		}
		out = append(out, r)
	}
	return out
}
