// Copyright 2020 The LUCI Authors.
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

package mask

import (
	"fmt"
	"strings"
	"unicode"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// path models the parsed path which consists of a slice of segments
type path []string

var pathDelimiter = '.'

// Parse a path string to a slice of segments (See grammer in pkg doc).
//
// Removes the trailing stars if presents, e.g. parses ["a.*"] as ["a"].
//
// If isJSONName is true, parsing the field name using json field name instead
// of its canonical form. However, the result segments in path will use
// canonical field name.
func parsePath(rawPath string, descriptor protoreflect.MessageDescriptor, isJSONName bool) (path, error) {
	t := &tokenizer{
		path:      []rune(rawPath),
		delimiter: pathDelimiter,
	}
	ctx := &parseCtx{
		curDescriptor: descriptor,
	}
	ret := path{}
	for t.hasMoreTokens() {
		if tok, err := t.nextToken(); err != nil {
			return nil, err
		} else {
			seg, err := parseSegment(tok, isJSONName, ctx)
			if err != nil {
				return nil, err
			}
			ret = append(ret, seg)
		}
	}

	if length := len(ret); ret[length-1] == "*" {
		return ret[:length-1], nil
	}
	return ret, nil
}

// Context of parsing the path
type parseCtx struct {
	curDescriptor protoreflect.MessageDescriptor
	isList        bool
	mustBeLast    bool
}

// advanceToField advance the conext to next field of the current message.
// Returns the canonical form of field name. Returns error when the supplied
// field doesn't exist in message or the current message descriptor is nil
//(meaning scalar field).
//
// If isJSONName is true, we will assume the given field name is JSON name and
// look up the JSON name instead of the field name.
func (ctx *parseCtx) advanceToField(fieldName string, isJSONName bool) (string, error) {
	if msgDesc := ctx.curDescriptor; msgDesc != nil {
		var fieldDesc protoreflect.FieldDescriptor
		if isJSONName {
			fieldDesc = msgDesc.Fields().ByJSONName(fieldName)
		} else {
			fieldDesc = msgDesc.Fields().ByName(protoreflect.Name(fieldName))
		}
		if fieldDesc == nil {
			return "", fmt.Errorf("field %s does not exist in message %s", fieldName, msgDesc.Name())
		}
		ctx.curDescriptor = fieldDesc.Message()
		ctx.isList = fieldDesc.IsList()
		return string(fieldDesc.Name()), nil
	}
	return "", fmt.Errorf("can't advance to field when current descriptor is nil")
}

// mapKeyKindToTokenType defines the mapping between the kind of mapkey
// and the expected token type of the token in path string
var mapKeyKindToTokenType = map[protoreflect.Kind]tokenType{
	protoreflect.Int32Kind:    intLiteral,
	protoreflect.Int64Kind:    intLiteral,
	protoreflect.Sint32Kind:   intLiteral,
	protoreflect.Sint64Kind:   intLiteral,
	protoreflect.Uint32Kind:   intLiteral,
	protoreflect.Uint64Kind:   intLiteral,
	protoreflect.Sfixed32Kind: intLiteral,
	protoreflect.Fixed32Kind:  intLiteral,
	protoreflect.Sfixed64Kind: intLiteral,
	protoreflect.Fixed64Kind:  intLiteral,
	protoreflect.BoolKind:     boolLiteral,
	protoreflect.StringKind:   strLiteral,
}

// parseSegment parses a token to a segment string and update the prase context
// accordingly.
//
// If isJSONName is true, the token value is expected to be JSON name of
// a field of a message instead of canonical name. However, the return segment
// will always be canonical name.
func parseSegment(tok token, isJSONName bool, ctx *parseCtx) (string, error) {
	switch desc := ctx.curDescriptor; {
	case ctx.mustBeLast:
		return "", fmt.Errorf("expect end of string; got token: %s", tok.value)
	case ctx.isList:
		// The current segment corresponds to a list field (non-map entry repeated field). Only star is allowed
		if tok.typ != star {
			return "", fmt.Errorf("expect a star following repeated field; got token: %s", tok.value)
		}
		ctx.isList = false
		return "*", nil

	case desc == nil:
		return "", fmt.Errorf("scalar field cannot have subfield: %s", tok.value)

	case desc.IsMapEntry():
		if tok.typ != star {
			keyKind := desc.Fields().ByName(protoreflect.Name("key")).Kind()
			if expectTokenType, found := mapKeyKindToTokenType[keyKind]; !found {
				return "", fmt.Errorf("unexpected map key kind %s", keyKind.String())
			} else if expectTokenType != tok.typ {
				return "", fmt.Errorf("expect map key kind %s; got token: %s", keyKind.String(), tok.value)
			}
		}

		if _, err := ctx.advanceToField("value", false); err != nil {
			return "", err
		}
		return tok.value, nil

	case tok.typ == star:
		// a star cannot be followed by any subfields if it does not corresponds to a repeated field
		ctx.mustBeLast = true
		return "*", nil

	case tok.typ != strLiteral:
		return "", fmt.Errorf("expect a field name of type string; got token: %s", tok.value)

	default:
		return ctx.advanceToField(tok.value, isJSONName)
	}
}

// tokenizer breaks a path string into tokens(segments)
type tokenizer struct {
	path      []rune
	delimiter rune
	pos       int
}

// token is a composite of token type and the raw string value. It represents
// a segment in the path
type token struct {
	typ   tokenType
	value string
}

// tokenType models different types of segment defined in grammer (see pkg doc).
// Note that, quoted string will also be treated as string literal.
type tokenType int8

const (
	star tokenType = iota + 1
	strLiteral
	boolLiteral
	intLiteral
)

// String returns the name of types of token.
func (t tokenType) String() string {
	switch t {
	case star:
		return "star"
	case strLiteral:
		return "string"
	case boolLiteral:
		return "boolean"
	case intLiteral:
		return "integer"
	default:
		return fmt.Sprintf("TokenType(%d)", t)
	}
}

// hasMoreTokens tests if there are more tokens available from the path string
func (t tokenizer) hasMoreTokens() bool {
	return t.pos < len(t.path)
}

// nextToken returns the next token in the path string
func (t *tokenizer) nextToken() (token, error) {
	if t.pos > 0 {
		// if not reading the first token, expecting a delimiter
		if t.path[t.pos] != t.delimiter {
			return token{}, fmt.Errorf("expect delimiter: %c; got %c", t.delimiter, t.path[t.pos])
		}
		t.pos++ // swallow the delimiter
		if t.pos == len(t.path) {
			return token{}, fmt.Errorf("path can't end with delimiter: %c", t.delimiter)
		}
	}

	switch r, pathLen := t.path[t.pos], len(t.path); {
	case r == '`':
		t.pos++ // swallow the starting backtick
		var sb strings.Builder
		for {
			val := read(t.path[t.pos:], "", func(r rune) bool { return r != '`' })
			t.pos += len(val)
			sb.WriteString(val)
			if t.pos >= pathLen {
				return token{}, fmt.Errorf("a quoted string is never closed; got: %s", sb.String())
			}
			t.pos++ // Swallow the discovered backtick
			if t.pos >= pathLen || (t.pos < pathLen && t.path[t.pos] != '`') {
				// Stop if eof or the discovered backtick is not for escaping
				break
			}
			sb.WriteRune('`')
			t.pos++ // Swallow the escaped backtick
		}
		return token{
			typ:   strLiteral,
			value: sb.String(),
		}, nil

	case r == '*':
		t.pos++ // swallow the star
		return token{
			typ:   star,
			value: "*",
		}, nil

	case r == '-' || unicode.IsDigit(r):
		val := read(t.path[t.pos+1:], string(r), unicode.IsDigit)
		t.pos += len(val)
		return token{
			typ:   intLiteral,
			value: val,
		}, nil

	case r == '_' || unicode.IsLetter(r):
		val := read(t.path[t.pos+1:], string(r), stringPredicate)
		t.pos += len(val)
		typ := strLiteral
		if val == "true" || val == "false" {
			typ = boolLiteral
		}
		return token{
			typ:   typ,
			value: val,
		}, nil

	default:
		return token{}, fmt.Errorf("unknown token: %c", r)
	}
}

// stringPredicate return true if the given unicode code point matches
// the string literal grammar
var stringPredicate = func(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// read scans the provided unicode code points from the beginning until the
// condition is not matched. Returns all matched code points as a string
// with given prefix
func read(data []rune, prefix string, cond func(r rune) bool) string {
	var sb strings.Builder
	sb.WriteString(prefix)
	for _, r := range data {
		if cond(r) {
			sb.WriteRune(r)
		} else {
			break
		}
	}
	return sb.String()
}
