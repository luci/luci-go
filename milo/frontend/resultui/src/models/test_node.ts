// Copyright 2020 The LUCI Authors.
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

import { action, computed, observable } from 'mobx';

import { TestVariant } from '../services/resultdb';


/**
 * Regex for extracting segments from a test ID.
 */
// Use /[a-zA-Z0-9_-]*([^a-zA-Z0-9_-]|$)/g instead of
// /[a-zA-Z0-9_-]+([^a-zA-Z0-9_-]|$)/g so testIds ending with /[^a-zA-Z0-9_-]/
// will get their own leaves.
// This ensures only leaf nodes can have directly associated tests.
// Without this, nodes may be incorrectly elided when there's a testId that
// ends with /[^a-zA-Z0-9_-]/.
// For example, when we add 'parent:' and 'parent:child' to the tree,
// 'child' will be incorrectly elided into 'parent:',
// even though 'parent:' contains two different testIds.
export const ID_SEG_REGEX = /[a-zA-Z0-9_-]*([^a-zA-Z0-9_-]|$)/g;

/**
 * Contains test results and exonerations, grouped by variant,
 * for the given test id.
 */
export interface ReadonlyTest {
  readonly id: string;
  readonly variants: readonly TestVariant[];
}

/**
 * TestNode enables the organization of tests into a tree-like structure,
 * branching on path components in the test name.
 * Path components are delimited by non-alphanumeric characters (i.e. /\W/).
 */
export class TestNode {
  // The path leads to this node, including the name of this node.
  // Can be used as an identifier of this node.
  private readonly unelidedPath: string;
  @observable.shallow private readonly unelidedChildrenMap = new Map<string, TestNode>();
  @computed private get unelidedChildren() {
    return [...this.unelidedChildrenMap.values()].sort((v1, v2) => {
      return v1.unelidedName.localeCompare(v2.unelidedName);
    });
  }

  // The properties belongs to the elided node
  // (descendants with no siblings are elided into this node).
  @computed get path() { return this.elidedNode.node.unelidedPath; }
  @computed get name() { return this.elidedNode.name; }
  @computed get children(): readonly TestNode[] { return this.elidedNode.node.unelidedChildren; }
  @computed private get elidedNode() {
    let name = this.unelidedName;

    // If the node has a single child, elide it into its parent.
    let node: TestNode = this;
    while (node.unelidedChildrenMap.size === 1) {
      node = node.unelidedChildrenMap.values().next().value;
      name += node.unelidedName;
    }
    return {name, node};
  }

  static newRoot() { return new TestNode('', ''); }
  private constructor(prefix: string, private readonly unelidedName: string) {
    this.unelidedPath = prefix + unelidedName;
  }

  /**
   * Takes a test and adds it to the appropriate place in the tree,
   * creating new nodes as necessary.
   * @param testId test.id must be alphabetically greater than the id of any
   *     previously added test.
   */
  @action
  addTestId(testId: string) {
    const idSegs = testId.match(ID_SEG_REGEX)!;
    idSegs.reverse();
    this.addTestIdSegs(idSegs);
  }

  private addTestIdSegs(idSegStack: string[]) {
    const nextSeg = idSegStack.pop();
    if (nextSeg === undefined) {
      return;
    }
    let child = this.unelidedChildrenMap.get(nextSeg);
    if (child === undefined) {
      child = new TestNode(this.unelidedPath, nextSeg);
      this.unelidedChildrenMap.set(nextSeg, child);
    }
    child.addTestIdSegs(idSegStack);
  }
}
