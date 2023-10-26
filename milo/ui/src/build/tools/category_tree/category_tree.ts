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
   * Perform a depth-first traversal and yield all items in the leaves.
   *
   * For example, traversing the following tree
   * ```
   *       root
   *       /  \
   *      d    a
   *     / \    \
   *    e   b    f
   *   /     \    \
   * item1    c  item3
   *         /
   *       item2
   * ```
   * will yield items: item1, item2, item3.
   */
  items(): Iterable<CategorizedItem<T>>;
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
 * The children of any branch are sorted by the order they are first discovered.
 * For example, building a tree from the following items:
 *  * item1 with category: d > e
 *  * item2 with category: d > b > c
 *  * item3 with category: a > f
 *
 * will result in a tree that looks like the graph below
 * ```
 *       root
 *       /  \
 *      d    a
 *     / \    \
 *    e   b    f
 *   /     \    \
 * item1    c  item3
 *         /
 *       item2
 * ```
 */
export function buildCategoryTree<T>(
  items: ReadonlyArray<CategorizedItem<T>>,
): CategoryTree<T> {
  const root = new CategoryTreeBranch<T>();
  for (const item of items) {
    root.addItem(item, 0);
  }
  return root;
}

class CategoryTreeBranch<T> implements CategoryTree<T> {
  private children: Array<CategoryTreeBranch<T> | CategoryTreeLeaf<T>> = [];
  private branchMap: { [key: string]: CategoryTreeBranch<T> } = {};

  addItem(item: CategorizedItem<T>, depth: number) {
    if (item.category.length <= depth) {
      this.children.push(new CategoryTreeLeaf(item));
      return;
    }

    const key = item.category[depth];
    let branch = this.branchMap[key];
    if (!branch) {
      branch = new CategoryTreeBranch();
      this.branchMap[key] = branch;
      this.children.push(branch);
    }

    branch.addItem(item, depth + 1);
  }

  *items(): Iterable<CategorizedItem<T>> {
    for (const child of this.children) {
      yield* child.items();
    }
  }
}

class CategoryTreeLeaf<T> implements CategoryTree<T> {
  constructor(readonly item: CategorizedItem<T>) {}

  *items(): Iterable<CategorizedItem<T>> {
    yield this.item;
  }
}
