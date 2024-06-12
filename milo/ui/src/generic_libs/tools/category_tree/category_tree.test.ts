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

import { CategoryTree, CategoryTreeEntry } from './category_tree';

const entries: readonly CategoryTreeEntry<string, number>[] = [
  [['cat1B', 'cat2A'], 1],
  [['cat1A', 'cat2B'], 2],
  [['cat1B', 'cat2A'], 3],
  [['cat1A'], 4],
  [[], 5],
  [['cat1C', 'cat2B'], 6],
  [['cat1C', 'cat2A'], 7],
  [['cat1C', 'cat2B'], 8],
  [['cat1C', 'cat2B', 'cat3A'], 9],
];

describe('CategoryTree', () => {
  it('values should perform depth-first pre-order traversal', () => {
    const tree = new CategoryTree(entries);
    expect([...tree.values()]).toEqual([
      // Can handle item with empty category. It sits at the root so it's
      // yielded immediately.
      5,
      // Lifted because the category was previously inserted with value 1.
      3,
      // Lifted forward because it's in the ancestor node of value 2.
      4,
      // Not lifted forward because 'cat1A' is inserted later than 'cat1B',
      // even though 'cat1A' is alphanumerically smaller than 'cat1B'.
      2,
      // Lifted because the category was previously inserted with value 6.
      8,
      //
      9,
      // 'cat1C' -> 'cat2A' is not treated the same as 'cat1A' -> 'cat2A'.
      // Although 'cat2A' under 'cat1B' was inserted quite early,
      // 'cat1C' -> 'cat2A' is inserted later than 'cat1C' -> 'cat2B'.
      7,
    ]);
  });

  it('entries should generate the correct index', () => {
    const tree = new CategoryTree(entries);
    expect([...tree.enumerate()]).toEqual([
      // Can handle item with empty category. It sits at the root so it's
      // yielded immediately.
      [[], 5],
      // Lifted forward because the category was previously inserted with
      // value 1.
      [[0, 0], 3],
      // Lifted forward because it's in the ancestor node of value 2.
      [[1], 4],
      // Not lifted forward because 'cat1A' is inserted later than 'cat1B',
      // even though 'cat1A' is alphanumerically smaller than 'cat1B'.
      [[1, 0], 2],
      // Lifted forward because the category was previously inserted with
      // value 6.
      [[2, 0], 8],
      //
      [[2, 0, 0], 9],
      // 'cat1C' -> 'cat2A' is not treated the same as 'cat1A' -> 'cat2A'.
      // Although 'cat2A' under 'cat1B' was inserted quite early,
      // 'cat1C' -> 'cat2A' is inserted later than 'cat1C' -> 'cat2B'.
      [[2, 1], 7],
    ]);
  });

  describe('delete', () => {
    it('can delete existing descendant', () => {
      const tree = new CategoryTree(entries);
      expect(tree.delete(['cat1C', 'cat2B'])).toBe(true);
      expect([...tree.enumerate()]).toEqual([
        [[], 5],
        [[0, 0], 3],
        [[1], 4],
        [[1, 0], 2],
        // Deleted.
        // [[2, 0], 8],
        [[2, 0, 0], 9],
        [[2, 1], 7],
      ]);
    });

    it("return false when descendant doesn't exist", () => {
      const tree = new CategoryTree(entries);
      expect(tree.delete(['cat1C', 'cat2D'])).toBe(false);
      expect([...tree.enumerate()]).toEqual([
        [[], 5],
        [[0, 0], 3],
        [[1], 4],
        [[1, 0], 2],
        [[2, 0], 8],
        [[2, 0, 0], 9],
        [[2, 1], 7],
      ]);
    });

    it('can delete value from node with children', () => {
      const tree = new CategoryTree(entries);
      expect(tree.delete(['cat1A'])).toBe(true);
      expect([...tree.enumerate()]).toEqual([
        [[], 5],
        [[0, 0], 3],
        // Deleted.
        // [[1], 4],
        [[1, 0], 2],
        [[2, 0], 8],
        [[2, 0, 0], 9],
        [[2, 1], 7],
      ]);
    });

    it('does not delete node with children', () => {
      const tree = new CategoryTree(entries);
      expect(tree.delete(['cat1C'])).toBe(false);
      expect([...tree.enumerate()]).toEqual([
        [[], 5],
        [[0, 0], 3],
        [[1], 4],
        [[1, 0], 2],
        [[2, 0], 8],
        [[2, 0, 0], 9],
        [[2, 1], 7],
      ]);
    });

    it('reassign indices when a branch is deleted', () => {
      const tree = new CategoryTree(entries);
      expect(tree.delete(['cat1A'])).toBe(true);
      expect([...tree.enumerate()]).toEqual([
        [[], 5],
        [[0, 0], 3],
        [[1, 0], 2],
        [[2, 0], 8],
        [[2, 0, 0], 9],
        [[2, 1], 7],
      ]);
      expect(tree.delete(['cat1A', 'cat2B'])).toBe(true);
      expect([...tree.enumerate()]).toEqual([
        [[], 5],
        [[0, 0], 3],
        [[1, 0], 8],
        [[1, 0, 0], 9],
        [[1, 1], 7],
      ]);
    });
  });

  describe('probe', () => {
    it('can probe self', () => {
      const tree = new CategoryTree(entries);
      const [node, remainingKeys] = tree.probe([]);
      expect(node).toBe(tree);
      expect(remainingKeys).toStrictEqual([]);
    });

    it('can probe descendant', () => {
      const tree = new CategoryTree(entries);
      const [node, remainingKeys] = tree.probe(['cat1C', 'cat2B']);
      expect([...node.enumerate()]).toEqual([
        [[], 8],
        [[0], 9],
      ]);
      expect(remainingKeys).toStrictEqual([]);
    });

    it("can probe category that doesn't exist", () => {
      const tree = new CategoryTree(entries);
      const [node, remainingKeys] = tree.probe(['cat1C', 'cat2D']);
      expect([...node.enumerate()]).toEqual([
        [[0], 8],
        [[0, 0], 9],
        [[1], 7],
      ]);
      expect(remainingKeys).toStrictEqual(['cat2D']);
    });
  });

  describe('getDescendant', () => {
    it('can get self', () => {
      const tree = new CategoryTree(entries);
      expect(tree.getDescendant([])).toBe(tree);
    });

    it('can get existing descendant', () => {
      const tree = new CategoryTree(entries);
      expect([...tree.getDescendant(['cat1C', 'cat2B'])!.values()]).toEqual([
        8, 9,
      ]);
    });

    it("return undefined when descendant doesn't exist", () => {
      const tree = new CategoryTree(entries);
      expect(tree.getDescendant(['cat1C', 'cat2D'])).toBeUndefined();
    });

    it('can be chained', () => {
      const tree = new CategoryTree(entries);
      expect([
        ...tree
          .getDescendant([])!
          .getDescendant(['cat1C'])!
          .getDescendant([])!
          .getDescendant(['cat2B'])!
          .values(),
      ]).toEqual([8, 9]);
    });
  });
});
