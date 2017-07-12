// Copyright 2016 The LUCI Authors.
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

package nestedflagset

import (
	"bytes"
	"unicode/utf8"
)

// Context is the lexer's current state.
type lexerContext struct {
	value string
	index int
	delim rune
}

// nextToken parses and returns the next token in the string.
func (l *lexerContext) nextToken() token {
	buf := new(bytes.Buffer)

	index := 0
	escaped := false
	quoted := false

MainLoop:
	for _, c := range l.value[l.index:] {
		index += utf8.RuneLen(c)

		if escaped {
			escaped = false
			buf.WriteRune(c)
			continue
		}

		switch c {
		case '\\':
			escaped = true

		case '"':
			quoted = !quoted

		case l.delim:
			if quoted {
				buf.WriteRune(c)
			} else {
				break MainLoop
			}

		default:
			buf.WriteRune(c)
		}
	}

	l.index += index
	return token(buf.Bytes())
}

// Lexer creates a new lexer lexerContext.
func lexer(value string, delim rune) *lexerContext {
	return &lexerContext{
		value: value,
		index: 0,
		delim: delim,
	}
}

// finished returns whether the context is finished parsing.
func (l *lexerContext) finished() bool {
	return l.index == len(l.value)
}

// split splits the Lexer's string into a slice of Tokens.
func (l *lexerContext) split() []token {
	result := make([]token, 0, 16)
	for !l.finished() {
		result = append(result, l.nextToken())
	}
	return result
}
