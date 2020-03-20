/* Copyright 2020 The LUCI Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { computed, observable } from 'mobx';

import { TestExoneration, TestResult, Variant } from '../services/resultdb';

/**
 * TestResult or TestExoneration.
 */
export type TestResultOrExoneration = TestResult | TestExoneration;

/**
 * Tests if a TestResultOrExoneration is a test result.
 */
export function isTestResult(testResultOrExoneration: TestResultOrExoneration): testResultOrExoneration is TestResult {
  return (testResultOrExoneration as TestResult).resultId !== undefined;
}

/**
 * Contains test results and exonerations, grouped by variant,
 * for the given test id.
 */
export interface ReadonlyTest {
  readonly id: string;
  readonly variants: ReadonlyArray<ReadonlyVariant>;
}

/**
 * Contains test results and exonerations for the given test variant.
 */
export interface ReadonlyVariant {
  readonly variant: Variant;
  readonly results: ReadonlyArray<TestResult>;
  readonly exonerations: ReadonlyArray<TestExoneration>;
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
  @observable.shallow private unelidedChildren: TestNode[] = [];

  // The properties belongs to the elided node
  // (descendants with no siblings are elided into this node).
  @computed get path() { return this.elidedNode.node.unelidedPath; }
  @computed get branchName() { return this.elidedNode.branchName; }
  @computed get children(): readonly TestNode[] { return this.elidedNode.node.unelidedChildren; }
  @computed private get elidedNode() {
    let branchName = this.unelidedBranchName;

    // If the node has a single child, elide it into its parent.
    let node: TestNode = this;
    while (node.unelidedChildren.length === 1) {
      node = node.unelidedChildren[0];
      branchName += node.unelidedBranchName;
    }
    return {branchName, node};
  }

  static newRoot() { return new TestNode('', ''); }
  private constructor(prefix: string, private readonly unelidedBranchName: string) {
    this.unelidedPath = prefix + unelidedBranchName;
  }

  /**
   * Contains all tests belonging to this node and its descendants.
   */
  @computed get tests(): readonly ReadonlyTest[] { return this._tests; }
  // Use @observable instead of @computed so the time complexity is low when
  // accessed by non-observers.
  @observable.shallow private _tests: ReadonlyTest[] = [];

  /**
   * Takes a test and adds it to the appropriate place in the tree,
   * creating new nodes as necessary.
   */
  addTest(test: ReadonlyTest) {
    // Use /\w*(\W+|$)/g instead of /\w+(\W+|$)/g so testIds ending with a
    // non-alphanumerical character will get their own leaf.
    // This ensures only leaf nodes can have directly associated tests.
    // Without this, nodes may be incorrectly elided when there's a testId that
    // ends with /\W/.
    // For example, when we add 'parent:' and 'parent:child' to the tree,
    // 'child' will be incorrectly elided into 'parent:',
    // even though 'parent:' contains two different testIds.
    const idSegs = test.id.match(/\w*(\W+|$)/g)!;
    idSegs.reverse();
    this.addTestWithIdSegs(test, idSegs);
  }

  private addTestWithIdSegs(test: ReadonlyTest, idSegStack: string[]) {
    this._tests.push(test);
    const nextSeg = idSegStack.pop();
    if (nextSeg === undefined) {
      return;
    }
    // TODO(weiweilin): we will only need to compare with the last child once
    // all tests are sorted by testId.
    // crbug.com/1062117
    let child = this.unelidedChildren.find((c) => c.unelidedBranchName === nextSeg);
    if (child === undefined) {
      child = new TestNode(this.unelidedPath, nextSeg);
      this.unelidedChildren.push(child);
    }
    child.addTestWithIdSegs(test, idSegStack);
  }
}
