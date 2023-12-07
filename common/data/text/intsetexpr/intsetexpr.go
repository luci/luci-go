// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package intsetexpr implements parsing of expressions like `a{01..03}b`.
//
// It knows how to expand `a{01..03}b` into `[a01b, a02b, a03b]`.
package intsetexpr

import (
	"fmt"
	"strconv"
	"strings"
)

// Expand expands a string with a int set into a list of strings.
//
// For example, given `a{1..3}b` produces `['a1b', 'a2b', 'a3b']`.
//
// The incoming string should have no more than one `{...}` section. If it's
// absent, the function returns the list that contains one item: the original
// string.
//
// The set is given as comma-separated list of terms. Each term is either
// a single non-negative integer (e.g. `9`) or a range (e.g. `1..5`). Both ends
// of the range are inclusive. Ranges where the left hand side is larger than
// the right hand side are not allowed. All elements should be listed in the
// strictly increasing order (e.g. `1,2,5..10` is fine, but `5..10,1,2` is
// not). Spaces are not allowed.
//
// The output integers are padded with zeros to match the width of
// corresponding terms. For ranges this works only if both sides have same
// width. For example, `01,002,03..04` will expand into `01, 002, 03, 04`.
//
// Use `{{` and `}}` to escape `{` and `}` respectively.
func Expand(s string) ([]string, error) {
	// Fast path for strings that do not have sets at all.
	if !strings.ContainsAny(s, "{}") {
		return []string{s}, nil
	}

	// States for the parser state machine.
	const (
		StateBeforeLB = iota // scanning to find '{'
		StateAfterRB         // after {...} block is read, scanning for end

		// In comments below '|' denotes the position of the state machine.

		StateRangeStart  // '{|1..4,5}' or '{|1,2}', expecting to read a number or '}'
		StateCommaOrDots // '{1|..4,5}' or '{1|,2}, expecting either ',' or '..', or '}'
		StateRangeEnd    // '{1..|4,5}', expecting to read a number
		StateComma       // '{1..4|,5}', expecting ',' or '}'
	)

	// Represents e.g. "10..20", or just "10" if l == r
	type rnge struct {
		l, r uint64
		fmt  string // either %d or e.g. %03d
	}

	var ranges []rnge     // all read ranges
	var total int         // total number of output strings to expect
	var rangeStart string // for currently constructed range

	// addRange parses strings into ints and verifies ranges are in the increasing
	// order. 'r' is empty for single-element terms e.g. "{2}".
	addRange := func(l, r string) error {
		li, err := strconv.ParseUint(l, 10, 64)
		if err != nil {
			return fmt.Errorf("integer %q is too large", l)
		}

		var ri uint64
		if r != "" {
			if ri, err = strconv.ParseUint(r, 10, 64); err != nil {
				return fmt.Errorf("integer %q is too large", r)
			}
			// E.g. "5..2" is a bad range, should be "2..5". Same for "2..2".
			if li >= ri {
				return fmt.Errorf("bad range - %d is not larger than %d", ri, li)
			}
		} else {
			// For e.g. "{2}".
			ri = li
			r = l
		}

		// E.g. "10,9" is bad, should be "9,10". Same for "9,9".
		if len(ranges) > 0 {
			if min := ranges[len(ranges)-1].r; min >= li {
				return fmt.Errorf("the set is not in increasing order - %d is not larger than %d", li, min)
			}
		}

		// If both strings have the same length, use it as padding for the output.
		format := "%d"
		if len(l) == len(r) {
			format = fmt.Sprintf("%%0%dd", len(l))
		}

		ranges = append(ranges, rnge{li, ri, format})
		total += int(ri-li) + 1
		return nil
	}

	pfx := "" // everything before '{'
	sfx := "" // everything after '}'

	state := StateBeforeLB

	for _, tok := range tokenize(s) {
		switch state {
		case StateBeforeLB:
			switch tok.typ {
			case TokLB:
				state = StateRangeStart
			case TokRB:
				return nil, fmt.Errorf(`bad expression - "}" must appear after "{"`)
			default:
				pfx += tok.val
			}

		case StateAfterRB:
			switch tok.typ {
			case TokLB, TokRB:
				return nil, fmt.Errorf(`bad expression - only one "{...}" section is allowed`)
			default:
				sfx += tok.val
			}

		case StateRangeStart:
			switch tok.typ {
			case TokNum:
				rangeStart = tok.val
				state = StateCommaOrDots
			case TokRB:
				state = StateAfterRB
			default:
				return nil, fmt.Errorf(`bad expression - expecting a number or "}", got %q`, tok.val)
			}

		case StateCommaOrDots:
			switch tok.typ {
			case TokComma:
				if err := addRange(rangeStart, ""); err != nil {
					return nil, err
				}
				state = StateRangeStart
			case TokRB:
				if err := addRange(rangeStart, ""); err != nil {
					return nil, err
				}
				state = StateAfterRB
			case TokDots:
				state = StateRangeEnd
			default:
				return nil, fmt.Errorf(`bad expression - expecting ",", ".." or "}", got %q`, tok.val)
			}

		case StateRangeEnd:
			switch tok.typ {
			case TokNum:
				if err := addRange(rangeStart, tok.val); err != nil {
					return nil, err
				}
				state = StateComma
			default:
				return nil, fmt.Errorf(`bad expression - expecting a number, got %q`, tok.val)
			}

		case StateComma:
			switch tok.typ {
			case TokComma:
				state = StateRangeStart
			case TokRB:
				state = StateAfterRB
			default:
				return nil, fmt.Errorf(`bad expression - expecting "," or "}", got %q`, tok.val)
			}

		default:
			panic("impossible")
		}
	}

	if len(ranges) == 0 {
		return []string{pfx + sfx}, nil
	}

	out := make([]string, 0, total)
	for _, rng := range ranges {
		for i := rng.l; i <= rng.r; i++ {
			out = append(out, fmt.Sprintf("%s"+rng.fmt+"%s", pfx, i, sfx))
		}
	}
	return out, nil
}

////////////////////////////////////////////////////////////////////////////////
// Tokenizer.

const (
	TokLB    = iota // non-escaped '{'
	TokRB           // non-escaped '}'
	TokNum          // a sequence of digits
	TokRunes        // an arbitrary sequence of non-special runes
	TokComma        // ','
	TokDots         // '..'
)

type token struct {
	typ int    // one of TOK_* constants
	val string // substring the token was parsed from
}

func tokenize(s string) (out []token) {
	rs := []rune(s)

	emit := func(tok int, val string) {
		out = append(out, token{tok, val})
	}

	for i := 0; i < len(rs); i++ {
		// Advances 'i' util rs[i] matches the predicate.
		readUntil := func(pred func(r rune) bool) string {
			start := i
			for i < len(rs) && pred(rs[i]) {
				i++
			}
			i-- // overstepped
			return string(rs[start : i+1])
		}

		switch {
		case rs[i] == '{':
			// Escaped '{'?
			if i != len(rs)-1 && rs[i+1] == '{' {
				emit(TokRunes, "{")
				i++ // consumed already
			} else {
				emit(TokLB, "{")
			}
		case rs[i] == '}':
			// Escaped '}'?
			if i != len(rs)-1 && rs[i+1] == '}' {
				emit(TokRunes, "}")
				i++ // consumed already
			} else {
				emit(TokRB, "}")
			}
		case rs[i] == ',':
			emit(TokComma, ",")
		case rs[i] == '.':
			// ".."?
			if i != len(rs)-1 && rs[i+1] == '.' {
				emit(TokDots, "..")
				i++ // consumed already
			} else {
				emit(TokRunes, ".") // regular single dot
			}
		case rs[i] >= '0' && rs[i] <= '9':
			emit(TokNum, readUntil(func(r rune) bool {
				return r >= '0' && r <= '9'
			}))
		default:
			emit(TokRunes, readUntil(func(r rune) bool {
				special := r == '{' ||
					r == '}' ||
					r == ',' ||
					r == '.' ||
					(r >= '0' && r <= '9')
				return !special
			}))
		}
	}

	return
}
