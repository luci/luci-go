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

import {
  parseCommaSeparatedText,
  formatCommaSeparatedText,
} from './mrt_filter_menu_item_utils';

describe('parseCommaSeparatedText', () => {
  it('should parse simple comma separated values', () => {
    expect(parseCommaSeparatedText('a,b,c')).toEqual(['a', 'b', 'c']);
  });

  it('should trim whitespace around values', () => {
    expect(parseCommaSeparatedText(' a , b , c ')).toEqual(['a', 'b', 'c']);
  });

  it('should handle quoted values containing commas', () => {
    expect(parseCommaSeparatedText('"a,b",c')).toEqual(['a,b', 'c']);
  });

  it('should strip surrounding quotes from parsed values', () => {
    expect(parseCommaSeparatedText('"a",b')).toEqual(['a', 'b']);
  });

  it('should return empty array for empty string', () => {
    expect(parseCommaSeparatedText('')).toEqual([]);
  });

  it('should filter out empty parts', () => {
    expect(parseCommaSeparatedText('a,,b')).toEqual(['a', 'b']);
  });

  it('should handle irregular quoting robustly', () => {
    // FSM groups irregular quotes correctly avoiding unbounded splits
    expect(parseCommaSeparatedText('a,"b,c')).toEqual(['a', 'b,c']);
  });

  describe('formatCommaSeparatedText', () => {
    it('should format simple array', () => {
      expect(formatCommaSeparatedText(['a', 'b', 'c'])).toBe('a, b, c');
    });

    it('should quote values containing commas', () => {
      expect(formatCommaSeparatedText(['a,b', 'c'])).toBe('"a,b", c');
    });

    it('should handle Set input', () => {
      expect(formatCommaSeparatedText(new Set(['a', 'b']))).toBe('a, b');
    });

    it('should return empty string for empty input', () => {
      expect(formatCommaSeparatedText([])).toBe('');
    });
  });
});
