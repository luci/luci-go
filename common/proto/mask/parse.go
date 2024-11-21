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

const pathDelimiter = '.'

// parsePath parses a path string to a slice of segments (see grammar in pkg
// doc).
func parsePath(rawPath string, descriptor protoreflect.MessageDescriptor, allowAdvanced bool) (path, error) {
	return parsePathWithContext(rawPath, &parseCtx{
		curDescriptor: descriptor,
		allowAdvanced: allowAdvanced,
	})
}

func parsePathWithContext(rawPath string, ctx *parseCtx) (path, error) {
	t := &tokenizer{
		path:      rawPath,
		delimiter: pathDelimiter,
	}
	ret := path{}
	for t.hasMoreTokens() {
		if tok, err := t.nextToken(); err != nil {
			return nil, err
		} else {
			if !ctx.allowAdvanced && tok.isAdvancedSyntax() {
				return nil, fmt.Errorf("using unsupported advanced syntax")
			}
			seg, err := parseSegment(tok, ctx)
			if err != nil {
				return nil, err
			}
			ret = append(ret, seg)
		}
	}

	return ret, nil
}

// parseCtx defines context during path parsing
type parseCtx struct {
	curDescriptor protoreflect.MessageDescriptor
	allowAdvanced bool
	isList        bool
	mustBeLast    bool
}

// advanceToField advances the context to the next field of current message.
// Returns the canonical form of field name. Returns error when the supplied
// field doesn't exist in message or the current message descriptor is nil
// (meaning scalar field).
func (ctx *parseCtx) advanceToField(fieldName string) (string, error) {
	msgDesc := ctx.curDescriptor
	if msgDesc == nil {
		return "", fmt.Errorf("can't advance to field when current descriptor is nil")
	}
	fieldDesc := msgDesc.Fields().ByName(protoreflect.Name(fieldName))
	if fieldDesc == nil {
		return "", fmt.Errorf("field %q does not exist in message %s", fieldName, msgDesc.Name())
	}
	ctx.curDescriptor = fieldDesc.Message()
	ctx.isList = fieldDesc.IsList()
	return string(fieldDesc.Name()), nil
}

// mapKeyKindToTokenType defines the mapping between the kind of mapkey
// and the expected token type of the token in path string.
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

// parseSegment parses a token to a segment string and updates the parse context
// accordingly.
func parseSegment(tok token, ctx *parseCtx) (string, error) {
	switch desc := ctx.curDescriptor; {
	case ctx.mustBeLast:
		return "", fmt.Errorf("expected end of string; got token: %q", tok.value)
	case ctx.isList:
		// The current segment corresponds to a list field (non-map entry repeated
		// field). Only star is allowed
		if tok.typ != star {
			return "", fmt.Errorf("expected a star following a repeated field; got token: %q", tok.value)
		}
		ctx.isList = false
		return "*", nil

	case desc == nil:
		return "", fmt.Errorf("scalar field cannot have subfield: %q", tok.value)

	case desc.IsMapEntry():
		if tok.typ != star {
			keyKind := desc.Fields().ByName(protoreflect.Name("key")).Kind()
			if expectTokenType, found := mapKeyKindToTokenType[keyKind]; !found {
				return "", fmt.Errorf("unexpected map key kind %s", keyKind)
			} else if expectTokenType != tok.typ {
				return "", fmt.Errorf("expected map key kind %s; got token: %q", keyKind, tok.value)
			}
		}

		if _, err := ctx.advanceToField("value"); err != nil {
			return "", err
		}
		return tok.value, nil

	case tok.typ == star:
		// a star cannot be followed by any subfields if it does not corresponds to
		// a repeated field
		ctx.mustBeLast = true
		return "*", nil

	case tok.typ != strLiteral:
		return "", fmt.Errorf("expected a field name of type string; got token: %q", tok.value)

	default:
		return ctx.advanceToField(tok.value)
	}
}

// tokenizer breaks a path string into tokens(segments)
type tokenizer struct {
	path      string
	delimiter byte
	pos       int
}

// token is a composite of token type and the raw string value. It represents
// a segment in the path
type token struct {
	typ    tokenType
	value  string
	quoted bool
}

// isAdvancedSyntax is true if this token is not part of a standard grammar.
//
// In the standard grammar tokens are always just simple field names.
func (t *token) isAdvancedSyntax() bool {
	return t.typ != strLiteral || t.quoted
}

// tokenType models different types of segment defined in grammar (see pkg doc).
// Note that, quoted string will also be treated as string literal.
type tokenType int8

const (
	star tokenType = iota
	strLiteral
	boolLiteral
	intLiteral
)

// hasMoreTokens tests if there are more tokens available from the path string
func (t tokenizer) hasMoreTokens() bool {
	return t.pos < len(t.path)
}

// nextToken returns the next token in the path string. Always call
// hasMoreTokens before calling this function. Otherwise, This function call
// will panic with index out of range when there is no more token available.
func (t *tokenizer) nextToken() (token, error) {
	if t.pos > 0 {
		// if not reading the first token, expecting a delimiter
		if t.path[t.pos] != t.delimiter {
			return token{}, fmt.Errorf("expected delimiter: %c; got %c", t.delimiter, t.path[t.pos])
		}
		t.pos++ // swallow the delimiter
		if t.pos == len(t.path) {
			return token{}, fmt.Errorf("path can't end with delimiter: %c", t.delimiter)
		}
	}

	switch b, pathLen := t.path[t.pos], len(t.path); {
	case b == '`':
		t.pos++ // swallow the starting backtick
		sb := &strings.Builder{}
		for {
			nextBacktickRel := strings.IndexRune(t.path[t.pos:], '`')
			if nextBacktickRel == -1 {
				sb.WriteString(t.path[t.pos:])
				return token{}, fmt.Errorf("a quoted string is never closed; got: %q", sb)
			}
			nextBacktickAbs := t.pos + nextBacktickRel
			sb.WriteString(t.path[t.pos:nextBacktickAbs])
			t.pos = nextBacktickAbs + 1 // Swallow the discovered backtick as well
			if t.pos >= pathLen || t.path[t.pos] != '`' {
				// Stop if eof or the discovered backtick is not for escaping
				break
			}
			sb.WriteByte('`')
			t.pos++ // Swallow the escaped backtick
		}
		return token{
			typ:    strLiteral,
			value:  sb.String(),
			quoted: true,
		}, nil

	case b == '*':
		t.pos++ // swallow the star
		return token{
			typ:   star,
			value: "*",
		}, nil

	// Check if '-' is the last character or look ahead to see if it is followed
	// by a digit
	case b == '-' && (t.pos+1 == pathLen || !unicode.IsDigit(rune(t.path[t.pos+1]))):
		return token{}, fmt.Errorf("expected digit following minus sign for negative numbers; got minus sign only")

	case b == '-' || unicode.IsDigit(rune(b)):
		start := t.pos
		t.pos++ // swallow the first digit or minus sign felled through
		n := strings.IndexFunc(t.path[t.pos:], func(r rune) bool { return !unicode.IsDigit(r) })
		if n == -1 {
			t.pos = pathLen
		} else {
			t.pos += n
		}
		return token{
			typ:   intLiteral,
			value: t.path[start:t.pos],
		}, nil

	case b == '_' || unicode.IsLetter(rune(b)):
		start := t.pos
		t.pos++ // swallow the underscore or first letter
		n := strings.IndexFunc(t.path[t.pos:], isInvalidStringChar)
		if n == -1 {
			t.pos = pathLen
		} else {
			t.pos += n
		}
		typ, val := strLiteral, t.path[start:t.pos]
		if val == "true" || val == "false" {
			typ = boolLiteral
		}
		return token{
			typ:   typ,
			value: val,
		}, nil

	default:
		return token{}, fmt.Errorf("unexpected token: %c", b)
	}
}

// isInvalidStringChar tells whether the given rune represents an invalid
// character in a string literal according to the grammar.
func isInvalidStringChar(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
}
