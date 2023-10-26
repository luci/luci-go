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

import { buildCategoryTree } from './category_tree';

const items = [
  {
    category: ['cat1B', 'cat2A'],
    value: 1,
  },
  {
    category: ['cat1A', 'cat2B'],
    value: 2,
  },
  {
    category: ['cat1B', 'cat2A'],
    value: 3,
  },
  {
    category: ['cat1A'],
    value: 4,
  },
  {
    category: [],
    value: 5,
  },
  {
    category: ['cat1C', 'cat2B'],
    value: 6,
  },
  {
    category: ['cat1C', 'cat2A'],
    value: 7,
  },
  {
    category: ['cat1C', 'cat2B'],
    value: 8,
  },
  {
    category: ['cat1C', 'cat2B', 'cat3A'],
    value: 9,
  },
];

describe('CategoryTree', () => {
  it('should traverse the leaves in the right order', () => {
    const tree = buildCategoryTree(items);
    const values = [...tree.items()].map((i) => i.value);
    expect(values).toEqual([
      1,
      // Lifted forward because 'cat1B' is already discovered.
      3,
      // Not lifted forward because 'cat1A' is discovered later than 'cat1B',
      // even though 'cat1A' is alphanumerically smaller than 'cat1B'.
      2,
      //
      4,
      // Can handle item with empty category.
      5,
      //
      6,
      //
      8,
      //
      9,
      // 'cat1C' -> 'cat2A' is not treated the same as 'cat1A' -> 'cat2A'.
      // Although 'cat2A' under 'cat1B' was discovered quite early,
      // 'cat1C' -> 'cat2A' is discovered later than 'cat1C' -> 'cat2B'.
      7,
    ]);
  });
});
