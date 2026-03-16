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

import { Token, TokenKind } from '../token';

const LEXER_REGEXP = new RegExp(
  '^(?<comparator><=|>=|!=|<|>|=|:)|' +
    '(?<negateWord>NOT\\s)|' +
    '(?<negateSymbol>-)|' +
    '(?<and>AND\\s)|' +
    '(?<or>OR\\s)|' +
    '(?<dot>\\.)|' +
    '(?<lparen>\\()|' +
    '(?<rparen>\\))|' +
    '(?<comma>,)|' +
    '(?<string>"(?:[^"\\\\]|\\\\.)*")|' +
    '(?<text>[^\\s.,<>=!:\\(\\)]+)',
);

export class Lexer {
  private pos = 0;
  private nextToken: Token | null = null;

  constructor(private input: string) {}

  peek(): Token {
    if (!this.nextToken) this.nextToken = this.readNext();
    return this.nextToken;
  }

  next(): Token {
    const t = this.peek();
    this.nextToken = null;
    return t;
  }

  private readNext(): Token {
    // Skip whitespace
    const remaining = this.input.slice(this.pos);
    const whitespaceMatch = remaining.match(/^\s+/);
    if (whitespaceMatch) {
      this.pos += whitespaceMatch[0].length;
    }

    if (this.pos >= this.input.length) {
      return { kind: TokenKind.End, value: '' };
    }

    const current = this.input.slice(this.pos);
    const match = current.match(LEXER_REGEXP);

    if (!match || !match.groups) {
      throw new Error(`Unable to lex token at position ${this.pos}`);
    }

    this.pos += match[0].length;
    const groups = match.groups;

    if (groups.comparator)
      return { kind: TokenKind.Comparator, value: groups.comparator };
    if (groups.negateWord || groups.negateSymbol)
      return {
        kind: TokenKind.Negate,
        value: (groups.negateWord || groups.negateSymbol).trim(),
      };
    if (groups.and) return { kind: TokenKind.And, value: groups.and.trim() };
    if (groups.or) return { kind: TokenKind.Or, value: groups.or.trim() };
    if (groups.dot) return { kind: TokenKind.Dot, value: groups.dot };
    if (groups.lparen) return { kind: TokenKind.LParen, value: groups.lparen };
    if (groups.rparen) return { kind: TokenKind.RParen, value: groups.rparen };
    if (groups.comma) return { kind: TokenKind.Comma, value: groups.comma };
    if (groups.string) return { kind: TokenKind.String, value: groups.string };
    if (groups.text) return { kind: TokenKind.Text, value: groups.text };

    throw new Error(`Unhandled lexer match: ${match[0]}`);
  }
}
