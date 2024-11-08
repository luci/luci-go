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
  completionForPath, getContext, State, Token, tokenizeJSON, TokenKind,
} from './autocomplete';
import { Descriptors, FileDescriptorSet } from './prpc';
import { TestDescriptor } from './testdata/descriptor';

/* eslint-disable @typescript-eslint/no-non-null-assertion */

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

  it('completionForPath', () => {
    const descs = new Descriptors(TestDescriptor as FileDescriptorSet, []);
    const msg = descs.message('rpcexplorer.Autocomplete')!;

    const fields = (path: string): string[] => {
      const ctx = getContext(path);
      if (!ctx) {
        return [];
      }
      ctx.path.pop(); // the incomplete syntax element being edited
      const completion = completionForPath(msg, ctx.path);
      return completion ? completion.fields.map((f) => f.jsonName) : [];
    };

    const values = (path: string): string[] => {
      const ctx = getContext(path);
      if (!ctx) {
        return [];
      }
      const last = ctx.path.pop()!;
      const field = last.key?.val || '';
      const completion = completionForPath(msg, ctx.path);
      return completion ? completion.values(field).map((v) => v.value) : [];
    };

    expect(fields('{')).toEqual([
      'singleInt',
      'singleEnum',
      'singleMsg',
      'repeatedInt',
      'repeatedEnum',
      'repeatedMsg',
      'mapInt',
      'mapEnum',
      'mapMsg',
      'anyMsg',
      'durMsg',
      'tsMsg',
      'fmMsg',
      'structMsg',
      'strMsg',
      'intMsg',
    ]);

    expect(fields('{"singleMsg": {')).toEqual(['fooBar']);
    expect(fields('{"repeatedMsg": [{')).toEqual(['fooBar']);
    expect(fields('{"mapMsg": {0: {')).toEqual(['fooBar']);

    expect(fields('{"singleMsg": [{')).toEqual([]);
    expect(fields('{"repeatedMsg": {')).toEqual([]);
    expect(fields('{"repeatedMsg": [')).toEqual([]);
    expect(fields('{"mapMsg": [{')).toEqual([]);
    expect(fields('{"mapMsg": {')).toEqual([]);

    expect(fields('{"singleInt": {')).toEqual([]);
    expect(fields('{"repeatedInt": [{')).toEqual([]);
    expect(fields('{"mapInt": {0: {')).toEqual([]);

    expect(fields('{"missing": {')).toEqual([]);
    expect(fields('{"single_msg": {')).toEqual([]);

    expect(fields('{"anyMsg": {')).toEqual(['@type']); // synthetic field
    expect(fields('{"durMsg": {')).toEqual([]); // it is a string in JSON
    expect(fields('{"structMsg": {')).toEqual([]); // can be any JSON object
    expect(fields('{"strMsg": {')).toEqual([]); // it is a string in JSON

    expect(values('{"singleEnum": ')).toEqual(['V0', 'V1']);
    expect(values('{"repeatedEnum": [')).toEqual(['V0', 'V1']);
    expect(values('{"mapEnum": {0: ')).toEqual(['V0', 'V1']);

    expect(values('{"singleMsg": ')).toEqual([]);
    expect(values('{"repeatedMsg": ')).toEqual([]);
    expect(values('{"mapMsg": ')).toEqual([]);

    expect(values('{"durMsg": ')).toEqual(['1s']);
    expect(values('{"fmMsg": ')).toEqual(['field1.a.b.c,field2.a']);
    expect(values('{"tsMsg": ')).toHaveLength(1); // the current timestamp
  });
});
