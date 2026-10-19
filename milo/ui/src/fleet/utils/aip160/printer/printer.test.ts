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

import { parseFilter } from '../parser/parser';

import { printFilter } from './printer';

const mustParse = (input: string) => {
  const parsed = parseFilter(input);
  if (parsed.isError) {
    throw new Error(`Parse error: ${parsed.error}`);
  }
  return parsed.ast;
};

describe('printFilter', () => {
  it('returns empty string for null node or empty filter', () => {
    expect(printFilter(null)).toBe('');
    expect(printFilter(mustParse(''))).toBe('');
  });

  it('prints simple equality restrictions', () => {
    const input = 'model = "brya"';
    expect(printFilter(mustParse(input))).toBe('model = "brya"');
  });

  it('prints dot-qualified members with quotes and unquoted fields', () => {
    const input = 'labels.model = "brya"';
    expect(printFilter(mustParse(input))).toBe('labels.model = "brya"');

    const quotedMember = 'labels."dut_state" = "ready"';
    expect(printFilter(mustParse(quotedMember))).toBe(
      'labels."dut_state" = "ready"',
    );
  });

  it('prints AND conjunctions', () => {
    const input = 'model = "brya" AND pool = "DUT_POOL_QUOTA"';
    expect(printFilter(mustParse(input))).toBe(
      'model = "brya" AND pool = "DUT_POOL_QUOTA"',
    );
  });

  it('prints OR disjunctions', () => {
    const input = 'model = "brya" OR model = "corsola"';
    expect(printFilter(mustParse(input))).toBe(
      'model = "brya" OR model = "corsola"',
    );
  });

  it('prints parenthesized composite expressions', () => {
    const input =
      '(model = "brya" OR model = "corsola") AND pool = "DUT_POOL_QUOTA"';
    expect(printFilter(mustParse(input))).toBe(
      '(model = "brya" OR model = "corsola") AND pool = "DUT_POOL_QUOTA"',
    );
  });

  it('prints negated terms', () => {
    const input = '-model = "brya"';
    expect(printFilter(mustParse(input))).toBe('-model = "brya"');
  });

  it('prints has (:) operator without extra whitespace', () => {
    const input = 'labels.pool:*';
    expect(printFilter(mustParse(input))).toBe('labels.pool:*');
  });
});
