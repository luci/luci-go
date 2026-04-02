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

// This file contains a parser for AIP-160 filter expressions.
// The EBNF is at https://google.aip.dev/assets/misc/ebnf-filtering.txt
// The function call syntax is not supported which simplifies the parser.
//
// Implemented EBNF (in terms of lexer tokens):
// filter: [expression];
// expression: sequence {WS AND WS sequence};
// sequence: factor {WS factor};
// factor: term {WS OR WS term};
// term: [NEGATE] simple;
// simple: restriction | composite;
// restriction: comparable [COMPARATOR arg];
// comparable: member;
// member: (TEXT | STRING) {DOT (TEXT | STRING)};
// composite: LPAREN expression RPAREN;
// arg: comparable | composite;

import * as ast from '../ast/ast';
import { Lexer } from '../lexer/lexer';
import { TokenKind, Token } from '../token';

type Result =
  | {
      isError: false;
      ast: ast.Filter;
    }
  | { isError: true; error: string };

export function parseFilter(input: string): Result {
  const lexer = new Lexer(input);

  const peek = () => lexer.peek();
  const next = () => lexer.next();

  const accept = (kind: TokenKind): Token | null =>
    peek().kind === kind ? next() : null;

  const expect = (kind: TokenKind): Token => {
    const t = next();
    if (t.kind !== kind) throw new Error(`Expected ${kind} but got ${t.kind}`);
    return t;
  };

  // Parsing Grammar Functions
  const parseExpression = (): ast.Expression => {
    const sequences: ast.Sequence[] = [parseSequence()];
    while (accept(TokenKind.And)) {
      sequences.push(parseSequence());
    }
    return { kind: 'Expression', sequences };
  };

  const parseSequence = (): ast.Sequence => {
    const factors: ast.Factor[] = [];
    while (true) {
      const factor = parseFactor();
      if (!factor) break;
      factors.push(factor);
    }
    if (factors.length === 0)
      throw new Error('Expected at least one factor in sequence');
    return { kind: 'Sequence', factors };
  };

  const parseFactor = (): ast.Factor | null => {
    const term = parseTerm();
    if (!term) return null;
    const terms: ast.Term[] = [term];
    while (accept(TokenKind.Or)) {
      const nextTerm = parseTerm();
      if (!nextTerm) throw new Error('Expected term after OR');
      terms.push(nextTerm);
    }
    return { kind: 'Factor', terms };
  };

  const parseTerm = (): ast.Term | null => {
    const negated = !!accept(TokenKind.Negate);
    const simple = parseSimple();
    if (!simple) {
      if (negated) throw new Error('Expected simple term after negation');
      return null;
    }
    return { kind: 'Term', negated, simple };
  };

  const parseSimple = (): ast.Simple | null => {
    const restriction = parseRestriction();
    if (restriction) return restriction;

    if (accept(TokenKind.LParen)) {
      const expr = parseExpression();
      expect(TokenKind.RParen);
      return expr;
    }
    return null;
  };

  const parseRestriction = (): ast.Restriction | null => {
    const comp = parseComparable();
    if (!comp) return null;

    const comparator = accept(TokenKind.Comparator);
    if (!comparator) {
      return {
        kind: 'Restriction',
        comparable: comp,
        comparator: '',
        arg: null,
      };
    }

    const arg = parseArg();
    if (!arg) throw new Error(`Expected arg after ${comparator.value}`);
    return {
      kind: 'Restriction',
      comparable: comp,
      comparator: comparator.value,
      arg,
    };
  };

  const parseComparable = (): ast.Comparable | null => {
    const member = parseMember();
    return member ? { kind: 'Comparable', member } : null;
  };

  const parseMember = (): ast.Member | null => {
    const val = parseValue();
    if (!val) return null;
    const fields: ast.Value[] = [];
    while (accept(TokenKind.Dot)) {
      const field = parseValue();
      if (!field) throw new Error("Expected value after '.'");
      fields.push(field);
    }
    return { kind: 'Member', value: val, fields };
  };

  const parseValue = (): ast.Value | null => {
    let t = accept(TokenKind.String);
    if (t) {
      try {
        return { kind: 'Value', quoted: true, value: JSON.parse(t.value) };
      } catch {
        throw new Error(`Malformed string: ${t.value}`);
      }
    }
    t = accept(TokenKind.Text);
    return t ? { kind: 'Value', quoted: false, value: t.value } : null;
  };

  const parseArg = (): ast.Arg | null => {
    const comp = parseComparable();
    if (comp) return comp;
    if (accept(TokenKind.LParen)) {
      const expr = parseExpression();
      expect(TokenKind.RParen);
      return expr;
    }
    return null;
  };

  // Entry Point
  try {
    if (accept(TokenKind.End)) {
      return { isError: false, ast: { kind: 'Filter', expression: null } };
    }
    const expression = parseExpression();
    expect(TokenKind.End);
    return { isError: false, ast: { kind: 'Filter', expression } };
  } catch (e) {
    return {
      isError: true,
      error: e instanceof Error ? e.message : String(e),
    };
  }
}
