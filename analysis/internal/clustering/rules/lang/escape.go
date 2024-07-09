// Copyright 2022 The LUCI Authors.
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

package lang

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// regexpMetacharacters is the set of characters that have meaning (beyond
// the literal value) to the RE2 regular expression engine.
var regexpMetacharacters map[rune]struct{}

func init() {
	regexpMetacharacters = make(map[rune]struct{})
	for _, r := range `\.+*?()|[]{}^$` {
		regexpMetacharacters[r] = struct{}{}
	}
}

// likePatternToRegexp converts the given LIKE pattern to a corresponding
// RE2 regular expression pattern. The "%" and "_" tokens are encoded as
// ".*" and "." in the corresponding regex, unless they are escaped with
// a backslash "\" . Any regexp metacharacters in the input string
// are escaped to ensure they are not interpreted.
func likePatternToRegexp(likePattern string) (string, error) {
	var b strings.Builder
	// Set flags to let . match any character, including "\n".
	b.WriteString("(?s)")
	// Match start of string.
	b.WriteString("^")
	isEscaping := false
	for _, r := range likePattern {
		switch {
		case !isEscaping && r == '\\':
			isEscaping = true
		case !isEscaping && r == '%':
			b.WriteString(".*")
		case !isEscaping && r == '_':
			b.WriteString(".")
		case isEscaping && (r != '\\' && r != '%' && r != '_'):
			return "", fmt.Errorf(`unrecognised escape sequence in LIKE pattern "\%s"`, string(r))
		default: // !isEscaping || (isEscaping && (r == '\\' || r == '%' || r == '_'))
			// Match the literal character.
			if _, ok := regexpMetacharacters[r]; ok {
				// Escape regex metacharacters with a '\'.
				b.WriteRune('\\')
				b.WriteRune(r)
			} else {
				b.WriteRune(r)
			}
			isEscaping = false
		}
	}
	if isEscaping {
		return "", errors.New(`unfinished escape sequence "\" at end of LIKE pattern`)
	}
	// Match end of string.
	b.WriteString("$")
	return b.String(), nil
}

// ValidateLikePattern validates the given string is a valid LIKE
// pattern. In particular, this checks that all escape sequences
// are valid, and that there is no unfinished trailing escape
// sequence (trailing '\').
func ValidateLikePattern(likePattern string) error {
	_, err := likePatternToRegexp(likePattern)
	return err
}

// Matches double-quoted string literals supported by golang, which
// are a subset of those supported by Standard SQL. Handles standard escape
// sequences (\r, \n, etc.), plus octal, hex and unicode sequences.
// Refer to:
// https://golang.org/ref/spec#Rune_literals
// https://cloud.google.com/bigquery/docs/reference/standard-sql/lexical
// Single-quoted string literals are currently not supported.
const stringLiteralPattern = `"([^\\"]|\\[abfnrtv\\"]|\\[0-7]{3}|\\x[0-9a-fA-F]{2}|\\u[0-9a-fA-F]{4}|\\U[0-9a-fA-F]{8})*"`

// unescapeStringLiteral derives the unescaped string value from an escaped
// SQL string literal.
func unescapeStringLiteral(s string) (string, error) {
	// Don't allow the string literal to contain unescaped unicode replacement
	// characters (\ufffd, aka '�').
	// We don't like to see them in failure association rules as they suggest
	// the presence of Unicode bugs, even though the characters themselves
	// are valid unicode characters and may be needed in the rule to match
	// failure reasons which have the same characters in them.
	if strings.ContainsRune(s, utf8.RuneError) {
		return "", errors.New("string literal may not contain error rune directly (U+FFFD), use escape sequence '\\ufffd' instead")
	}

	// Interpret the string as a double-quoted go string
	// literal, decoding any escape sequences. Except for '\?' and
	// '\`', which are not supported in golang (but are not needed for
	// expressiveness), this matches the escape sequences in Standard SQL.
	// Refer to:
	// https://golang.org/ref/spec#Rune_literals
	// https://cloud.google.com/bigquery/docs/reference/standard-sql/lexical
	value, err := strconv.Unquote(s)
	if err != nil {
		// In most cases invalid strings should have already been
		// rejected by the lexer.
		return "", fmt.Errorf("invalid string literal: %s", s)
	}
	if !utf8.ValidString(value) {
		// Check string is UTF-8.
		// https://cloud.google.com/bigquery/docs/reference/standard-sql/data-types#string_type
		return "", fmt.Errorf("string literal is not valid UTF-8: %q", s)
	}
	return value, nil
}
