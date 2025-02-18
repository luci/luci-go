// Copyright 2025 The LUCI Authors.
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

import { Lexer, TokenKind } from './lexer';

describe('Lexer', () => {
  describe('getAllTokens', () => {
    it.each([
      '',
      'simple',
      ' wsBefore',
      'wsAfter ',
      ' wsAround ',
      '  wsAroundLong  ',
      '"string"',
      ' "string" ',
      '"ws string"',
      '"quoted\\"string"', // Escaped quote in string
      "'single quoted'", // Single quoted string
      '\'single then double"', // Invalid mixed single and double quote in string

      '-negated',
      'NOT negated',
      '- a', // negate is not followed by a simple

      'dash-separated-name',
      'term -negated-term',
      'implicit and',
      'explicit AND and',
      'test OR or',
      'implicit and AND explicit',
      'implicit with OR term AND explicit OR term',
      '(composite)',
      ' (composite) ',
      '( composite )',
      'value<21',
      'value < 21',
      'value<=21',
      'value>21',
      'value>=21',
      'value=21',
      'value!=21',
      'value:21',
      'value=(composite)',
      'member.field',
      'composite (expression)',
      'empty.func.call()',
      'func.call(arg1, "arg2")',
      'a:b:c', // Multiple colons
      'a.b.c', // Multiple dots
      'a=1 AND b="2" OR c!=3', // Complex query
      'a < 1 AND b > 2', // Comparison operators
      'a <= 1 AND b >= 2', // Less than or equal to/Greater than or equal to operators

      // Invalid, but test for handling
      ' - negated ', // negate is not followed by a simple
      ' ( ) ', // empty composite
      ' a AND', // expression with trailing AND
      'AND a', // expression with leading AND
      'AND', // expression with just an AND
      'AND AND', // expression with two ANDs
      ' a OR', // factor with trailing OR
      'OR a', // factor with leading OR
      'OR', // factor with just an OR
      'OR OR', // factor with two ORs
      '\\', // Invalid escape sequence
    ])('input: %s', (input) => {
      const lexer = new Lexer(input);
      const tokens = lexer.getAllTokens();
      expect(tokens).toMatchSnapshot();
    });
  });

  describe('getAtPosition', () => {
    it.each([
      ['|'],
      ['| value < 21  '],
      [' |value < 21  '],
      [' valu|e < 21  '],
      [' value| < 21  '],
      [' value |< 21  '],
      [' value <| 21  '],
      [' value < |21  '],
      [' value < 21 | '],
      [' value < 21  |'],
    ])('inputWithCursor: %s', (inputWithCursor) => {
      const input = inputWithCursor.replace('|', '');
      const cursorPos = inputWithCursor.indexOf('|');
      const lexer = new Lexer(input);
      const tokens = lexer.getAtPosition(cursorPos);
      expect(tokens).toMatchSnapshot();
    });

    it('cursor position out of bounds', () => {
      const input = ' value < 21 ';
      const lexer = new Lexer(input);
      expect(lexer.getAtPosition(-1)).toEqual([]);
      expect(lexer.getAtPosition(input.length + 1)).toEqual([]);
    });
  });

  describe('getBeforeIndex', () => {
    it.each([
      [' a AND b>c ', 0, false, undefined, undefined],
      [' a AND b>c ', 0, true, undefined, undefined],
      [' a AND b>c ', 1, false, undefined, undefined],
      [' a AND b>c ', 1, true, TokenKind.Whitespace, ' '],
      [' a AND b>c ', 3, false, TokenKind.QualifiedFieldOrValue, 'a'],
      [' a AND b>c ', 3, true, TokenKind.Whitespace, ' '],
      [' a AND b>c ', 7, false, TokenKind.Comparator, '>'],
      [' a AND b>c ', 7, true, TokenKind.Comparator, '>'],
      [' a AND b>c ', 100, false, TokenKind.QualifiedFieldOrValue, 'c'],
      [' a AND b>c ', 100, true, TokenKind.Whitespace, ' '],
    ])(
      'input: %s, index: %i, allowWS: %s',
      (input, index, allowWS, expectedKind, expectedValue) => {
        const lexer = new Lexer(input);
        const token = lexer.getBeforeIndex(index, allowWS);
        expect(token?.kind).toBe(expectedKind);
        expect(token?.value).toBe(expectedValue);
      },
    );
  });

  describe('getAfterIndex', () => {
    it.each([
      [' a AND b>c ', 3, false, TokenKind.QualifiedFieldOrValue, 'b'],
      [' a AND b>c ', 3, true, TokenKind.Whitespace, ' '],
      [' a AND b>c ', 5, false, TokenKind.Comparator, '>'],
      [' a AND b>c ', 5, true, TokenKind.Comparator, '>'],
      [' a AND b>c ', 7, false, undefined, undefined],
      [' a AND b>c ', 7, true, TokenKind.Whitespace, ' '],
      [' a AND b>c ', 100, false, undefined, undefined],
      [' a AND b>c ', 100, true, undefined, undefined],
    ])(
      'input: %s, index: %i, allowWS: %s',
      (input, index, allowWS, expectedKind, expectedValue) => {
        const lexer = new Lexer(input);
        const token = lexer.getAfterIndex(index, allowWS);
        expect(token?.kind).toBe(expectedKind);
        expect(token?.value).toBe(expectedValue);
      },
    );
  });
});
