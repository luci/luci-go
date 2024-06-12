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

export interface ReadonlyCategoryTree<K, V> {
  /**
   * Whether the node itself has an assigned value.
   *
   * Note that `this.value === undefined` does not mean the node has no value.
   * Because `V` could be `undefined`.
   */
  readonly hasValue: boolean;
  /**
   * The value of this node.
   */
  readonly value: V | undefined;
  /**
   * The direct children of the node.
   */
  readonly children: readonly ReadonlyCategoryTree<K, V>[];
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
  values(): IterableIterator<V>;
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
  enumerate(): IterableIterator<[index: readonly number[], value: V]>;
  /**
   * Get the last node when traversing the tree via the specified category.
   * Also return the remaining category keys that were not traversed due to
   * missing nodes.
   */
  probe(
    category: readonly K[],
  ): readonly [ReadonlyCategoryTree<K, V>, readonly K[]];
  /**
   * Get the descendant node representing the specified category.
   * If it doesn't exist, return `undefined`.
   */
  getDescendant(category: readonly K[]): ReadonlyCategoryTree<K, V> | undefined;
}

export type CategoryTreeEntry<K, T> = readonly [
  /**
   * A list of keys that represent the category of the item.
   * If a category is a prefix of another category, then it is considered a
   * parent of the other category (e.g. ['A', 'B'] and ['A', 'C'] are two
   * different categories sharing the same parent category ['A']).
   */
  category: readonly K[],
  value: T,
];

/**
 * A category tree.
 *
 * The children of any node are sorted by the order they are inserted.
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
export class CategoryTree<K, V> implements ReadonlyCategoryTree<K, V> {
  private _hasValue = false;
  private _value: V | undefined = undefined;
  private childMap = new Map<K, CategoryTree<K, V>>();

  /**
   * Build a category tree from the provided entries.
   */
  constructor(entries?: readonly CategoryTreeEntry<K, V>[] | null) {
    for (const [category, value] of entries || []) {
      this.set(category, value);
    }
  }

  /**
   * Set the value of the specified category.
   */
  set(category: readonly K[], value: V) {
    this.setImpl(category, value, 0);
    return this;
  }

  private setImpl(category: readonly K[], value: V, depth: number) {
    if (category.length <= depth) {
      this._value = value;
      this._hasValue = true;
      return;
    }

    const key = category[depth];
    let child = this.childMap.get(key);
    if (!child) {
      child = new CategoryTree();
      this.childMap.set(key, child);
    }

    child.setImpl(category, value, depth + 1);
  }

  /**
   * Delete the value of the specified category.
   *
   * Also remove the category branch leading to the deleted entry if the branch
   * has no other attached value.
   *
   * Return true if there's a value associated with the specified category.
   * Return false otherwise.
   */
  delete(category: readonly K[]): boolean {
    let deleteCandidateParent: CategoryTree<K, V> | null = null;
    let deleteCandidateKey: K | null = null;

    let pivot: CategoryTree<K, V> | undefined;
    // eslint-disable-next-line @typescript-eslint/no-this-alias
    pivot = this;
    for (const key of category) {
      const pivotParent: CategoryTree<K, V> = pivot;
      pivot = pivotParent.childMap.get(key);
      if (!pivot) {
        return false;
      }

      if (pivot.childMap.size + (pivot._hasValue ? 1 : 0) > 1) {
        // If the node has more than one value in itself and its descendants,
        // its ancestors and itself should not be deleted.
        deleteCandidateParent = null;
      } else if (!deleteCandidateParent) {
        // If the node does not have a value and no more than one child, it
        // might be eligible for deletion. But we want to delete the top most
        // ancestor that satisfy the same constant.
        deleteCandidateParent = pivotParent;
        deleteCandidateKey = key;
      }
    }

    if (deleteCandidateParent) {
      deleteCandidateParent.childMap.delete(deleteCandidateKey!);
      return true;
    }
    if (pivot._hasValue) {
      pivot._hasValue = false;
      pivot._value = undefined;
      return true;
    }

    return false;
  }

  /**
   * Cast the tree to a `ReadonlyCategoryTree<K, V>`.
   */
  asReadonly(): ReadonlyCategoryTree<K, V> {
    return this;
  }

  // Implements `ReadonlyCategoryTree<K, V>`.
  get hasValue(): boolean {
    return this._hasValue;
  }

  // Implements `ReadonlyCategoryTree<K, V>`.
  get value(): V | undefined {
    return this._value;
  }

  // Implements `ReadonlyCategoryTree<K, V>`.
  get children(): readonly CategoryTree<K, V>[] {
    return [...this.childMap.values()];
  }

  // Implements `ReadonlyCategoryTree<K, V>`.
  *values(): IterableIterator<V> {
    if (this._hasValue) {
      yield this._value!;
    }
    for (const child of this.childMap.values()) {
      yield* child.values();
    }
  }

  // Implements `ReadonlyCategoryTree<K, V>`.
  *enumerate(): IterableIterator<[index: readonly number[], value: V]> {
    for (const [rIndex, value] of this.entriesImpl()) {
      yield [rIndex.reverse(), value];
    }
  }

  private *entriesImpl(): IterableIterator<[rIndex: number[], V]> {
    if (this._hasValue) {
      yield [[], this._value!];
    }
    let i = 0;
    for (const child of this.childMap.values()) {
      for (const [rIndex, value] of child.entriesImpl()) {
        rIndex.push(i);
        yield [rIndex, value];
      }
      i += 1;
    }
  }

  // Implements `ReadonlyCategoryTree<K, V>`.
  probe(category: readonly K[]): readonly [CategoryTree<K, V>, readonly K[]] {
    // eslint-disable-next-line @typescript-eslint/no-this-alias
    let pivot: CategoryTree<K, V> = this;
    for (const [i, key] of category.entries()) {
      const next = pivot.childMap.get(key);
      if (!next) {
        return [pivot, category.slice(i)];
      }
      pivot = next;
    }
    return [pivot, []];
  }

  // Implements `ReadonlyCategoryTree<K, V>`.
  getDescendant(category: readonly K[]): CategoryTree<K, V> | undefined {
    const [node, remainingKeys] = this.probe(category);
    return remainingKeys.length ? undefined : node;
  }
}
