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

import { action, computed, observable } from 'mobx';

import { TestExoneration, TestResult, Variant } from '../services/resultdb';

/**
 * TestResult or TestExoneration
 */
export type TestObject = TestResult | TestExoneration;

/**
 * Tests if a TestObject is a test result.
 */
export function isTestResult(testObject: TestObject): testObject is TestResult {
  return (testObject as TestResult).resultId !== undefined;
}

/**
 * Contains test results and exonerations, grouped by variant, for the given test id.
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
 * TestNode enables the organization of tests into a tree-like structure, branching on path components in the test name.
 * Path components are delimited by non-alphanumeric characters (i.e. /\W/).
 * For efficiency, the tree representation is lazily evaluated. After tests are added to the node, the children of the node will not be computed until this.branchUnbranchedTests() is called.
 * this.branchUnbranchedTests() will only populate the direct children of the node, not the children's children.
 */
export class TestNode {
  private readonly branchName: string;
  // The path leads to this node, including the branchName of this node.
  readonly path: string;
  @observable.shallow private unbranchedTests: Array<{test: ReadonlyTest, idSegs: string[]}> = [];
  @computed get hasUnbranchedTest() { return this.unbranchedTests.length !== 0; }

  // Not up-to-date when there are unbranched test objects.
  @observable.shallow private tests: ReadonlyTest[] = [];
  @observable.shallow private children: TestNode[] = [];

  // elided... means the property belongs to the elided node (descendants with no siblings are elided into this node).
  // Not up-to-date when there are unbranched test objects.
  @computed get elidedBranchName() { return this.elidedNode.branchName; }
  @computed get elidedTests(): readonly ReadonlyTest[] { return this.elidedNode.node.elidedTests; }
  @computed get elidedChildren(): readonly TestNode[] { return this.elidedNode.node.children; }
  @computed private get elidedNode() {
    let branchName = this.branchName;

    // If the node has a single child, elide it into its parent.
    let node: TestNode = this;
    while (node.children.length === 1) {
      node = node.children[0];
      branchName += node.branchName;
    }
    return {branchName, node};
  }

  static newRoot() { return new TestNode('', ''); }
  private constructor(prefix: string, branchName: string) {
    this.branchName = branchName;
    this.path = prefix + branchName;
  }

  /**
   * Contains all tests belonging to this node and its decendents.
   * Always up-to-date (i.e. contains tests that haven't been lazily evaluated into tree form yet).
   */
  @computed get allTests(): readonly ReadonlyTest[] { return this._allTests; }
  @observable.shallow private _allTests: ReadonlyTest[] = [];


  /**
   * Adds a test to be processed.
   * The tree is not grown until this.processTests() is called.
   * @precondition prevTests.every((prev) => test.id.localeCompare(prev.id) >= 0)
   */
  addTest(test: ReadonlyTest) {
    // Use /\w*(\W|$)/g instead of /\w+(\W|$)/g so testId ends with a non-alphanumerical character will have its own leaf.
    // This ensures only leaf nodes can have directly associated tests.
    // Without this, nodes may be incorrectly elided when there's a testId ends whith /\W/.
    // For example, when we add 'parent:' and 'parent:child' to the tree, 'child' will be incorrectly elided into 'parent:',
    // even though 'parent:' contains two different testIds.
    const idSegs = test.id.match(/\w*(\W|$)/g)!.reverse();
    // /$/ is matched twice when testId ends with /\w/, pop it.
    if (idSegs.length >= 2 && /\w$/.test(idSegs[idSegs.length - 2])) {
      idSegs.pop();
    }
    this.addTestWithIdSegs({test, idSegs});
  }

  private addTestWithIdSegs(test: {test: ReadonlyTest, idSegs: string[]}) {
    this.unbranchedTests.push(test);
    this._allTests.push(test.test);
  }

  /**
   * Converts all unprocessed tests in this node into branches of this node.
   * Recursively calls this method on its desecedants if they are elided into this node.
   */
  @action
  branchUnbranchedTests() {
    for (const test of this.unbranchedTests) {
      this.branchTest(test);
    }
    if (this.children.length === 1 && this.unbranchedTests.length !== 0) {
      this.children[0].branchUnbranchedTests();
    }
    this.unbranchedTests = [];
  }

  /**
   * Takes a test and adds it to the appropriate place in the tree, creating new nodes as necessary.
   */
  private branchTest(test: {test: ReadonlyTest, idSegs: string[]}) {
    const nextSeg = test.idSegs.pop();
    // If there are no segments left, the test belongs to this node.
    if (nextSeg === undefined) {
      this.tests.push(test.test);
      return;
    }
    // TODO(weiweilin): we will only need to compare with the last child once all tests are sorted by testId.
    // crbug.com/1062117
    let child = this.children.find((c) => c.branchName === nextSeg);
    if (child === undefined) {
      child = new TestNode(this.path, nextSeg);
      this.children.push(child);
    }
    child.addTestWithIdSegs(test);
  }
}
