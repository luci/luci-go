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
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"text/template"

	"go.chromium.org/luci/starlark/builtins"
	"go.starlark.net/starlark"
	"gopkg.in/yaml.v2"
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

////////////////////////////////////////////////////////////////////////////////
// Text templates support.

type templateValue struct {
	tmpl *template.Template
	hash uint32
}

// String returns the string representation of the value.
func (t *templateValue) String() string { return "template(...)" }

// Type returns a short string describing the value's type.
func (t *templateValue) Type() string { return "template" }

// Freeze does nothing since templateValue is already immutable.
func (t *templateValue) Freeze() {}

// Truth returns the truth value of an object.
func (t *templateValue) Truth() starlark.Bool { return starlark.True }

// Hash returns a function of x such that Equals(x, y) => Hash(x) == Hash(y).
func (t *templateValue) Hash() (uint32, error) { return t.hash, nil }

// AttrNames returns all .<attr> of this object.
func (t *templateValue) AttrNames() []string {
	return []string{"render"}
}

// Attr returns a .<name> attribute of this object or nil.
func (t *templateValue) Attr(name string) (starlark.Value, error) {
	switch name {
	case "render":
		return templateRenderBuiltin.BindReceiver(t), nil
	default:
		return nil, nil
	}
}

// render implements template rendering using given value as input.
func (t *templateValue) render(data interface{}) (string, error) {
	buf := bytes.Buffer{}
	if err := t.tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Implementation of template.render(**dict) builtin.
var templateRenderBuiltin = starlark.NewBuiltin("render", func(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("render: expecting only keyword arguments, got %d positional", len(args))
	}

	// Convert kwargs to a real dict and then to a go nested map reusing to_json
	// machinery.
	data := starlark.NewDict(len(kwargs))
	for _, tup := range kwargs {
		if len(tup) != 2 {
			panic(fmt.Sprintf("impossible kwarg with len %d", len(tup)))
		}
		if err := data.SetKey(tup[0], tup[1]); err != nil {
			panic(fmt.Sprintf("impossible bad kwarg %s %s", tup[0], tup[1]))
		}
	}
	obj, err := builtins.ToGoNative(data)
	if err != nil {
		return nil, fmt.Errorf("render: %s", err)
	}

	out, err := fn.Receiver().(*templateValue).render(obj)
	if err != nil {
		return nil, fmt.Errorf("render: %s", err)
	}
	return starlark.String(out), nil
})

///

type templateCache struct {
	cache map[string]*templateValue // SHA256 of body => parsed template
}

func (tc *templateCache) get(body string) (starlark.Value, error) {
	hash := sha256.Sum256([]byte(body))
	cacheKey := string(hash[:])
	if t, ok := tc.cache[cacheKey]; ok {
		return t, nil
	}

	tmpl, err := template.New("<str>").Parse(body)
	if err != nil {
		return nil, err // note: the error is already prefixed by "template: ..."
	}
	val := &templateValue{tmpl: tmpl}

	fh := fnv.New32a()
	fh.Write([]byte(body))
	val.hash = fh.Sum32()

	if tc.cache == nil {
		tc.cache = make(map[string]*templateValue, 1)
	}
	tc.cache[cacheKey] = val
	return val, nil
}

////////////////////////////////////////////////////////////////////////////////
// Register all native calls, see //internal/strutil.star for usage.

func init() {
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

	declNative("json_to_yaml", func(call nativeCall) (starlark.Value, error) {
		var json starlark.String
		if err := call.unpack(1, &json); err != nil {
			return nil, err
		}
		var buf interface{}
		if err := yaml.Unmarshal([]byte(json.GoString()), &buf); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON as YAML: %s", err)
		}
		out, err := yaml.Marshal(buf)
		if err != nil {
			return nil, err
		}
		return starlark.String(out), nil
	})

	declNative("b64_encode", func(call nativeCall) (starlark.Value, error) {
		var s starlark.String
		if err := call.unpack(1, &s); err != nil {
			return nil, err
		}
		return starlark.String(base64.StdEncoding.EncodeToString([]byte(s.GoString()))), nil
	})

	declNative("b64_decode", func(call nativeCall) (starlark.Value, error) {
		var s starlark.String
		if err := call.unpack(1, &s); err != nil {
			return nil, err
		}
		raw, err := base64.StdEncoding.DecodeString(s.GoString())
		if err != nil {
			return nil, err
		}
		return starlark.String(string(raw)), nil
	})

	declNative("template", func(call nativeCall) (starlark.Value, error) {
		var body starlark.String
		if err := call.unpack(1, &body); err != nil {
			return nil, err
		}
		return call.State.templates.get(body.GoString())
	})
}
