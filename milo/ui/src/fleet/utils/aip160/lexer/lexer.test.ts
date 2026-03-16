// Copyright 2026 The LUCI Authors.
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

import * as token from '../token';

import { Lexer } from './lexer';

describe('Lexer', () => {
  test('TestTokenKinds', () => {
    const tests = [
      { input: '<= 10', kind: token.TokenKind.Comparator, value: '<=' },
      { input: '-file', kind: token.TokenKind.Negate, value: '-' },
      { input: 'NOT file', kind: token.TokenKind.Negate, value: 'NOT' },
      { input: 'AND b', kind: token.TokenKind.And, value: 'AND' },
      { input: 'OR a', kind: token.TokenKind.Or, value: 'OR' },
      { input: '.field', kind: token.TokenKind.Dot, value: '.' },
      { input: '."field"', kind: token.TokenKind.Dot, value: '.' },
      { input: '(arg)', kind: token.TokenKind.LParen, value: '(' },
      { input: ')', kind: token.TokenKind.RParen, value: ')' },
      { input: ', arg2)', kind: token.TokenKind.Comma, value: ',' },
      { input: 'text', kind: token.TokenKind.Text, value: 'text' },
      { input: '"string"', kind: token.TokenKind.String, value: '"string"' },
    ];

    tests.forEach((testCase) => {
      const l = new Lexer(testCase.input);
      const tok = l.next();
      expect(tok.kind).toBe(testCase.kind);
      expect(tok.value).toBe(testCase.value);
    });
  });

  test('TestWhitespaceLexing', () => {
    const filter =
      'text "string with whitespace" (43 AND 44) OR 45 NOT function(arg1, arg2):hello -field1.field2: hello field < 36';
    const tokens = [
      { kind: token.TokenKind.Text, value: 'text' },
      { kind: token.TokenKind.String, value: '"string with whitespace"' },
      { kind: token.TokenKind.LParen, value: '(' },
      { kind: token.TokenKind.Text, value: '43' },
      { kind: token.TokenKind.And, value: 'AND' },
      { kind: token.TokenKind.Text, value: '44' },
      { kind: token.TokenKind.RParen, value: ')' },
      { kind: token.TokenKind.Or, value: 'OR' },
      { kind: token.TokenKind.Text, value: '45' },
      { kind: token.TokenKind.Negate, value: 'NOT' },
      { kind: token.TokenKind.Text, value: 'function' },
      { kind: token.TokenKind.LParen, value: '(' },
      { kind: token.TokenKind.Text, value: 'arg1' },
      { kind: token.TokenKind.Comma, value: ',' },
      { kind: token.TokenKind.Text, value: 'arg2' },
      { kind: token.TokenKind.RParen, value: ')' },
      { kind: token.TokenKind.Comparator, value: ':' },
      { kind: token.TokenKind.Text, value: 'hello' },
      { kind: token.TokenKind.Negate, value: '-' },
      { kind: token.TokenKind.Text, value: 'field1' },
      { kind: token.TokenKind.Dot, value: '.' },
      { kind: token.TokenKind.Text, value: 'field2' },
      { kind: token.TokenKind.Comparator, value: ':' },
      { kind: token.TokenKind.Text, value: 'hello' },
      { kind: token.TokenKind.Text, value: 'field' },
      { kind: token.TokenKind.Comparator, value: '<' },
      { kind: token.TokenKind.Text, value: '36' },
      { kind: token.TokenKind.End, value: '' },
      { kind: token.TokenKind.End, value: '' },
    ];

    const l = new Lexer(filter);
    tokens.forEach((expected) => {
      const actual = l.next();
      expect(actual.kind).toBe(expected.kind);
      expect(actual.value).toBe(expected.value);
    });
  });
});
