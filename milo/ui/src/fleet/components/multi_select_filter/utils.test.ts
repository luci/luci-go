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

import { fuzzySort } from './utils';

describe('multi_select_filter/utils', () => {
  describe('fuzzySort', () => {
    it('find a perfect match', () => {
      const options = ['abc', 'def', 'ghi'];
      const query = options[1];

      const result = fuzzySort(query)(options);
      expect(result).toEqual([options[1]]);
    });

    it('be empty if there are no matches', () => {
      const options = ['abc', 'def', 'ghi'];
      const query = 'zzz';

      const result = fuzzySort(query)(options);
      expect(result).toEqual([]);
    });

    it('find an option with a missing character', () => {
      const options = ['abc', 'def', 'ghi'];
      const query = 'ab';

      const result = fuzzySort(query)(options);
      expect(result).toContain(options[0]);
    });

    it('priorities consecutive matches', () => {
      const options = ['azzza', 'aaa', 'bb'];
      const query = 'aa';

      const result = fuzzySort(query)(options);
      expect(result).toEqual(['aaa', 'azzza']);
    });

    it('works with a getter function', () => {
      const options = [-2, 3, 0, 1, 2];
      const query = 'a';
      const getter = (n: number) => String.fromCharCode('a'.charCodeAt(0) + n);

      const result = fuzzySort(query)(options, getter);
      expect(result).toEqual([0]);
    });

    it('respects minScore', () => {
      const options = ['abc', 'def', 'ghi'];
      const query = options[1];

      const result = fuzzySort(query, Infinity)(options);
      expect(result).toEqual([]);
    });

    it('works with nested options', () => {
      const options = [['abc', 'def'], ['ghi']];
      const query = 'ab';

      const result = fuzzySort(query)(options);
      expect(result).toContain(options[0]);
    });
  });
});
