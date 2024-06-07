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

export interface CategoryTree<T> {
  /**
   * The direct children of the node.
   */
  readonly children: readonly CategoryTree<T>[];
  /**
   * Performs a depth-first pre-order traversal and yield all values in the
   * nodes.
   *
   * For example, traversing the following tree
   * ```
   * // Only alphabetic letters are nodes. Numeric numbers are values attached
   * // to the node.
   *
   *       root
   *      /    \
   *     d      a
   *    / \    / \
   *   e   b  f   4
   *  /     \  \
   * 1      c   3
   *       /
   *      2
   * ```
   * will yield values: 1, 2, 4, 3.
   */
  values(): Iterable<T>;
  /**
   * Performs a depth-first pre-order traversal and yield all values in the
   * nodes along with the index path of their category.
   *
   * For example, traversing the following tree.
   * ```ascii
   * // Only alphabetic letters are nodes. Numeric numbers are values attached
   * // to the node.
   *
   *       root
   *      /    \
   *     d      a
   *    / \    / \
   *   e   b  f   4
   *  /     \  \
   * 1      c   3
   *       /
   *      2
   * ```
   * will yield entries:
   *  * index: [0, 0]     value: 1
   *  * index: [0, 1, 0]  value: 2
   *  * index: [1]        value: 4
   *  * index: [1, 0]     value: 3
   */
  entries(): Iterable<[index: readonly number[], value: T]>;
  /**
   * Get the descendant node representing the specified category.
   * If it doesn't exist, return `undefined`.
   */
  getDescendant(category: readonly string[]): CategoryTree<T> | undefined;
}

export interface CategorizedItem<T> {
  /**
   * A list of keys that represent the category of the item.
   * If a category is a prefix of another category, then it is considered a
   * parent of the other category (e.g. ['A', 'B'] and ['A', 'C'] are two
   * different categories sharing the same parent category ['A']).
   */
  readonly category: readonly string[];
  readonly value: T;
}

/**
 * Build a category tree from the provided categorized items.
 *
 * The children of any node are sorted by the order they are first discovered.
 * For example, building a tree from the following items:
 *  * item1 with category: d > e
 *  * item2 with category: d > b > c
 *  * item3 with category: a > f
 *  * item4 with category: a
 *
 * will result in a tree that looks like the graph below
 * ```
 * // Only alphabetic letters are nodes. Numeric numbers are values attached
 * // to the node.
 *
 *       root
 *      /    \
 *     d      a
 *    / \    / \
 *   e   b  f   4
 *  /     \  \
 * 1      c   3
 *       /
 *      2
 * ```
 */
export function buildCategoryTree<T>(
  items: ReadonlyArray<CategorizedItem<T>>,
): CategoryTree<T> {
  const root = new CategoryTreeNode<T>();
  for (const item of items) {
    root.addValue(item.value, item.category, 0);
  }
  return root;
}

class CategoryTreeNode<T> implements CategoryTree<T> {
  private selfValues: T[] = [];
  private _children: Array<CategoryTreeNode<T>> = [];
  private childMap: { [key: string]: CategoryTreeNode<T> } = {};

  addValue(value: T, category: readonly string[], depth: number) {
    if (category.length <= depth) {
      this.selfValues.push(value);
      return;
    }

    const key = category[depth];
    let child = this.childMap[key];
    if (!child) {
      child = new CategoryTreeNode();
      this.childMap[key] = child;
      this._children.push(child);
    }

    child.addValue(value, category, depth + 1);
  }

  get children(): readonly CategoryTree<T>[] {
    return this._children;
  }

  *values(): Iterable<T> {
    yield* this.selfValues;
    for (const child of this._children) {
      yield* child.values();
    }
  }

  *entries(): Iterable<[index: readonly number[], value: T]> {
    for (const [rIndex, value] of this.entriesImpl()) {
      yield [rIndex.reverse(), value];
    }
  }

  *entriesImpl(): Iterable<[rIndex: number[], T]> {
    for (const value of this.selfValues) {
      yield [[], value];
    }
    for (const [i, child] of this._children.entries()) {
      for (const [rIndex, value] of child.entriesImpl()) {
        rIndex.push(i);
        yield [rIndex, value];
      }
    }
  }

  getDescendant(category: readonly string[]): CategoryTree<T> | undefined {
    // eslint-disable-next-line @typescript-eslint/no-this-alias
    let pivot: CategoryTreeNode<T> | undefined = this;
    for (const cat of category) {
      pivot = pivot.childMap[cat];
      if (!pivot) {
        return undefined;
      }
    }
    return pivot;
  }
}
