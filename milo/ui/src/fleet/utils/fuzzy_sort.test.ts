// Copyright 2024 The LUCI Authors.
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

import { fuzzySort } from './fuzzy_sort';

describe('fuzzy_sort', () => {
  describe('fuzzySort', () => {
    it('find a perfect match', () => {
      const options = ['abc', 'def', 'ghi'];
      const query = options[1];

      const result = fuzzySort(query)(options);
      expect(result[0].el).toEqual(options[1]);
    });

    it('returns same array if there are no matches', () => {
      const options = ['abc', 'def', 'ghi'];
      const query = 'zzz';

      const result = fuzzySort(query)(options);
      expect(result[0]).toEqual({
        el: options[0],
        score: -1,
        matches: [],
      });
      expect(result[1]).toEqual({
        el: options[1],
        score: -1,
        matches: [],
      });
      expect(result[2]).toEqual({
        el: options[2],
        score: -1,
        matches: [],
      });
    });

    it('find an option with a missing character', () => {
      const options = ['abc', 'def', 'ghi'];
      const query = 'ab';

      const result = fuzzySort(query)(options);
      expect(result.map((r) => r.el)).toContain(options[0]);
    });

    it('priorities consecutive matches', () => {
      const options = ['azzza', 'aaa', 'bb'];
      const query = 'aa';

      const result = fuzzySort(query)(options);
      expect(result.map((r) => r.el)).toEqual(['aaa', 'azzza', 'bb']);
    });

    it('priorities exact matches and matches with higher edge proximity', () => {
      const options = ['bidding', 'dut id', 'id'];
      const query = 'id';

      const result = fuzzySort(query)(options);
      expect(result.map((r) => r.el)).toEqual(['id', 'dut id', 'bidding']);
    });

    it('works with a getter function', () => {
      const options = [-2, 3, 0, 1, 2];
      const query = 'a';
      const getter = (n: number) => String.fromCharCode('a'.charCodeAt(0) + n);

      const result = fuzzySort(query)(options, getter);
      expect(result[0].el).toEqual(0);
    });

    it('works with nested options', () => {
      const options = [['abc', 'def'], ['ghi']];
      const query = 'ab';

      const result = fuzzySort(query)(options, String);
      expect(result.map((r) => r.el)).toContain(options[0]);
    });

    it('returns no matches with empty string', () => {
      const options = ['abcabc'];
      const query = '';

      const result = fuzzySort(query)(options);
      expect(result[0].matches).toEqual([]);
    });

    it('returns the correct matches', () => {
      const options = ['abcabc'];
      const query = 'ab';

      const result = fuzzySort(query)(options);
      expect(result[0].matches).toEqual([0, 1]);
    });

    it('finds scattered match when lookahead fails', () => {
      const options = ['axybczab'];
      const query = 'abc';

      const result = fuzzySort(query)(options);
      expect(result[0].score).toBeGreaterThan(0);
    });

    it('does not grey out Ubuntu-22.04 when querying ubun', () => {
      const options = ['Ubuntu', 'Ubuntu-22.04'];
      const query = 'ubun';

      const result = fuzzySort(query)(options);

      // Mimic threshold logic from string_list_filter.tsx
      const scores = result.map((s) => s.score).filter((s) => s >= 0);
      const maxScore = Math.max(...scores);
      const significant = result.map((s) => ({
        ...s,
        isSignificant: scores.length === 0 || s.score >= maxScore * 0.8,
      }));

      const significantLabels = significant
        .filter((s) => s.isSignificant)
        .map((s) => s.el);

      expect(significantLabels).toContain('Ubuntu');
      expect(significantLabels).toContain('Ubuntu-22.04');
    });
  });
});
