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

import { BLANK_VALUE } from '@/fleet/constants/filters';

import { parseFilters, stringifyFilters } from './parser';

describe('stringifyFilters', () => {
  it('should stringify a single filter', () => {
    expect(stringifyFilters({ key: ['value'] })).toBe('key = "value"');
  });

  it('should stringify a multi-value filter', () => {
    expect(stringifyFilters({ key: ['v1', 'v2'] })).toBe(
      'key = ("v1" OR "v2")',
    );
  });

  it('should stringify a blank filter', () => {
    expect(stringifyFilters({ key: [BLANK_VALUE] })).toBe('NOT key');
  });

  it('should stringify a blank and single value filter', () => {
    expect(stringifyFilters({ key: [BLANK_VALUE, 'v1'] })).toBe(
      '(NOT key OR key = "v1")',
    );
  });

  it('should stringify a blank and multi-value filter', () => {
    expect(stringifyFilters({ key: [BLANK_VALUE, 'v1', 'v2'] })).toBe(
      '(NOT key OR key = ("v1" OR "v2"))',
    );
  });

  it('should stringify multiple filters', () => {
    expect(
      stringifyFilters({
        key1: ['v1'],
        key2: [BLANK_VALUE],
        key3: ['v2', 'v3'],
        key4: [BLANK_VALUE, 'v4'],
      }),
    ).toBe(
      'key1 = "v1" NOT key2 key3 = ("v2" OR "v3") (NOT key4 OR key4 = "v4")',
    );
  });
});

describe('parseFilters', () => {
  it('should parse a single filter', () => {
    expect(parseFilters('key = "value"').filters).toEqual({ key: ['value'] });
  });

  it('should parse a filters with key like labels.key quoting the key part', () => {
    expect(parseFilters('labels.key = "value"').filters).toEqual({
      'labels."key"': ['value'],
    });
  });

  it('should parse a multi-value filter', () => {
    expect(parseFilters('key = ("v1" OR "v2")').filters).toEqual({
      key: ['v1', 'v2'],
    });
  });

  it('should parse a blank filter', () => {
    expect(parseFilters('NOT key').filters).toEqual({ key: [BLANK_VALUE] });
  });

  it('should parse a blank and single value filter', () => {
    expect(parseFilters('(NOT key OR key = "v1")').filters).toEqual({
      key: [BLANK_VALUE, 'v1'],
    });
  });

  it('should parse a blank and multi-value filter', () => {
    expect(parseFilters('(NOT key OR key = ("v1" OR "v2"))').filters).toEqual({
      key: [BLANK_VALUE, 'v1', 'v2'],
    });
  });

  it('should parse multiple filters', () => {
    const result = parseFilters(
      'key1 = "v1" NOT key2 key3 = ("v2" OR "v3") (NOT key4 OR key4 = "v4")',
    );
    expect(result.error).toBeUndefined();
    expect(result.filters).toEqual({
      key1: ['v1'],
      key2: [BLANK_VALUE],
      key3: ['v2', 'v3'],
      key4: [BLANK_VALUE, 'v4'],
    });
  });

  it('should parse labels with weird characters', () => {
    const result = parseFilters('(NOT label OR label="this:\\"isNOT(weird)")');
    expect(result.error).toBeUndefined();
    expect(result.filters).toEqual({
      label: [BLANK_VALUE, 'this:\\"isNOT(weird)'],
    });
  });

  it('should return an error for invalid syntax', () => {
    expect(parseFilters('key = (v1 OR v2').error).toBeInstanceOf(Error);
    expect(parseFilters('(NOT key OR key = "v1"').error).toBeInstanceOf(Error);
    expect(parseFilters('key = ').error).toBeInstanceOf(Error);
  });
});
