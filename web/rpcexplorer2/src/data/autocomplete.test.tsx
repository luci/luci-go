// Copyright 2023 The LUCI Authors.
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

import {
  State, getContext, Token, tokenizeJSON, TokenKind,
} from './autocomplete';

describe('Autocomplete', () => {
  it('tokenizeJSON', () => {
    const testInput = `
      {}[]:,
      0123
      null true false
      "abc"
      "\\""
      "\\x22"
      "incomp
    ` + '\n\t\r ' + '"incomplete';

    const out: Token[] = [];
    tokenizeJSON(testInput, (tok) => out.push({
      kind: tok.kind,
      raw: tok.raw,
      val: tok.val,
    }));

    const token = (kind: TokenKind, raw: string, val?: string): Token => {
      return {
        kind: kind,
        raw: raw,
        val: val == undefined ? raw : val,
      };
    };

    expect(out).toEqual([
      token(TokenKind.Punctuation, '{'),
      token(TokenKind.Punctuation, '}'),
      token(TokenKind.Punctuation, '['),
      token(TokenKind.Punctuation, ']'),
      token(TokenKind.Punctuation, ':'),
      token(TokenKind.Punctuation, ','),
      token(TokenKind.EverythingElse, '0123'),
      token(TokenKind.EverythingElse, 'null'),
      token(TokenKind.EverythingElse, 'true'),
      token(TokenKind.EverythingElse, 'false'),
      token(TokenKind.String, '"abc"', 'abc'),
      token(TokenKind.String, '"\\""', '"'),
      token(TokenKind.BrokenString, '"\\x22"', ''),
      token(TokenKind.IncompleteString, '"incomp', 'incomp'),
      token(TokenKind.IncompleteString, '"incomplete', 'incomplete'),
    ]);
  });

  it('getContext', () => {
    const call = (text: string): [State, string[]] | undefined => {
      const ctx = getContext(text);
      if (ctx == undefined) {
        return undefined;
      }
      const path: string[] = [];
      for (const item of ctx.path) {
        let str = item.key == undefined ? '' : item.key.val;
        if (item.value != undefined) {
          str += ':' + item.value.val;
        }
        if (item.kind == 'list') {
          str = '[' + str + ']';
        }
        path.push(str);
      }
      return [ctx.state, path];
    };

    expect(call('"abc"')).toEqual([
      State.AfterValue,
      [':abc'],
    ]);

    expect(call('{')).toEqual([
      State.BeforeKey,
      ['', ''],
    ]);

    expect(call('{ "ab')).toEqual([
      State.InsideKey,
      ['', 'ab'],
    ]);

    expect(call('{ "abc"')).toEqual([
      State.AfterKey,
      ['', 'abc'],
    ]);

    expect(call('{ "abc":')).toEqual([
      State.BeforeValue,
      ['', 'abc'],
    ]);

    expect(call('{ "abc": {')).toEqual([
      State.BeforeKey,
      ['', 'abc', ''],
    ]);

    expect(call('{ "abc": [')).toEqual([
      State.BeforeValue,
      ['', 'abc', '[]'],
    ]);

    expect(call('{ "abc": "xy')).toEqual([
      State.InsideValue,
      ['', 'abc:xy'],
    ]);

    expect(call('{ "abc": {"1": "2", "3": "4"}')).toEqual([
      State.AfterValue,
      ['', 'abc'],
    ]);

    expect(call('{ "abc": {"1": "2", "3": "4"}, "xyz')).toEqual([
      State.InsideKey,
      ['', 'xyz'],
    ]);

    const broken = [
      ']',
      '}',
      '{{',
      '{[',
      '{]',
      '[}',
      '{,',
      '{ "abc",',
      '{ "abc" "def"',
      '{ "abc": "def" : ',
      '"abc",',
    ];
    for (const str of broken) {
      expect(call(str)).toBeUndefined();
    }
  });
});
