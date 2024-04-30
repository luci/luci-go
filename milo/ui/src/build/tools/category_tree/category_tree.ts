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
   * Perform a depth-first pre-order traversal and yield all values in the
   * nodes.
   *
   * For example, traversing the following tree
   * ```
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
  private children: Array<CategoryTreeNode<T>> = [];
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
      this.children.push(child);
    }

    child.addValue(value, category, depth + 1);
  }

  *values(): Iterable<T> {
    yield* this.selfValues;
    for (const child of this.children) {
      yield* child.values();
    }
  }
}
