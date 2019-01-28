// Copyright 2019 The LUCI Authors.
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

package lucicfg

import (
	"fmt"
	"strconv"
	"strings"

	"go.starlark.net/starlark"
)

// expandIntSet implements expand_int_set from //internal/strutil.star, see the
// doc there.
func expandIntSet(s string) ([]string, error) {
	// Fast path for strings that do not have sets at all.
	if !strings.ContainsAny(s, "{}") {
		return []string{s}, nil
	}

	// States for the parser state machine.
	const (
		BEFORE_LB = iota // scanning to find '{'
		AFTER_RB         // after {...} block is read, scanning for end

		// In comments below '|' denotes the position of the state machine.

		RANGE_START   // '{|1..4,5}' or '{|1,2}', expecting to read a number or '}'
		COMMA_OR_DOTS // '{1|..4,5}' or '{1|,2}, expecting either ',' or '..', or '}'
		RANGE_END     // '{1..|4,5}', expecting to read a number
		COMMA         // '{1..4|,5}', expecting ',' or '}'
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

	state := BEFORE_LB

	for _, tok := range tokenize(s) {
		switch state {
		case BEFORE_LB:
			switch tok.typ {
			case TOK_LB:
				state = RANGE_START
			case TOK_RB:
				return nil, fmt.Errorf(`bad expression - "}" must appear after "{"`)
			default:
				pfx += tok.val
			}

		case AFTER_RB:
			switch tok.typ {
			case TOK_LB, TOK_RB:
				return nil, fmt.Errorf(`bad expression - only one "{...}" section is allowed`)
			default:
				sfx += tok.val
			}

		case RANGE_START:
			switch tok.typ {
			case TOK_NUM:
				rangeStart = tok.val
				state = COMMA_OR_DOTS
			case TOK_RB:
				state = AFTER_RB
			default:
				return nil, fmt.Errorf(`bad expression - expecting a number or "}", got %q`, tok.val)
			}

		case COMMA_OR_DOTS:
			switch tok.typ {
			case TOK_COMMA:
				if err := addRange(rangeStart, ""); err != nil {
					return nil, err
				}
				state = RANGE_START
			case TOK_RB:
				if err := addRange(rangeStart, ""); err != nil {
					return nil, err
				}
				state = AFTER_RB
			case TOK_DOTS:
				state = RANGE_END
			default:
				return nil, fmt.Errorf(`bad expression - expecting ",", ".." or "}", got %q`, tok.val)
			}

		case RANGE_END:
			switch tok.typ {
			case TOK_NUM:
				if err := addRange(rangeStart, tok.val); err != nil {
					return nil, err
				}
				state = COMMA
			default:
				return nil, fmt.Errorf(`bad expression - expecting a number, got %q`, tok.val)
			}

		case COMMA:
			switch tok.typ {
			case TOK_COMMA:
				state = RANGE_START
			case TOK_RB:
				state = AFTER_RB
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
	TOK_LB    = iota // non-escaped '{'
	TOK_RB           // non-escaped '}'
	TOK_NUM          // a sequence of digits
	TOK_RUNES        // an arbitrary sequence of non-special runes
	TOK_COMMA        // ','
	TOK_DOTS         // '..'
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
				emit(TOK_RUNES, "{")
				i++ // consumed already
			} else {
				emit(TOK_LB, "{")
			}
		case rs[i] == '}':
			// Escaped '}'?
			if i != len(rs)-1 && rs[i+1] == '}' {
				emit(TOK_RUNES, "}")
				i++ // consumed already
			} else {
				emit(TOK_RB, "}")
			}
		case rs[i] == ',':
			emit(TOK_COMMA, ",")
		case rs[i] == '.':
			// ".."?
			if i != len(rs)-1 && rs[i+1] == '.' {
				emit(TOK_DOTS, "..")
				i++ // consumed already
			} else {
				emit(TOK_RUNES, ".") // regular single dot
			}
		case rs[i] >= '0' && rs[i] <= '9':
			emit(TOK_NUM, readUntil(func(r rune) bool {
				return r >= '0' && r <= '9'
			}))
		default:
			emit(TOK_RUNES, readUntil(func(r rune) bool {
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

func init() {
	// See //internal/strutil.star.
	declNative("expand_int_set", func(call nativeCall) (starlark.Value, error) {
		var s starlark.String
		if err := call.unpack(1, &s); err != nil {
			return nil, err
		}
		res, err := expandIntSet(s.GoString())
		if err != nil {
			return nil, fmt.Errorf("expand_int_set: %s", err)
		}
		out := make([]starlark.Value, len(res))
		for i, r := range res {
			out[i] = starlark.String(r)
		}
		return starlark.NewList(out), nil
	})
}
