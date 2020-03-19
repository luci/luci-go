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
 *  Test if a TestObject is a test result.
 */
export function isTestResult(testObject: TestObject): testObject is TestResult {
  return (testObject as TestResult).resultId !== undefined;
}

/**
 * Represents test objects of the given id grouped by variant.
 */
export interface ReadonlyTest {
  readonly id: string;
  readonly variants: ReadonlyArray<ReadonlyVariant>;
}

/**
 * Test objects of the given id and variant.
 */
export interface ReadonlyVariant {
  readonly variant: Variant;
  readonly results: ReadonlyArray<TestResult>;
  readonly exonerations: ReadonlyArray<TestExoneration>;
}

/**
 * Store test in a tree like structure.
 * Node with no siblings are elided into their parents.
 * The tree representation is lazily evaluated.
 * After the tests are added to the node, the children of the node will not be computed
 * until this.branchUnbranchedTests() is called.
 * this.branchUnbranchedTests() will only populate the children of the node but not children's children.
 */
export class TestNode {
  // immediate... means the property directly belong to this node (descendents are not elided into this node).
  private readonly immediateBranchName: string;
  readonly immediatePath: string;
  @observable.shallow immediateUnbranchedTests: Array<{test: ReadonlyTest, idSegs: string[]}> = [];

  // Not up-to-date when there's unprocessed test objects.
  @observable.shallow private immediateTests: ReadonlyTest[] = [];
  @observable.shallow private immediateChildren: TestNode[] = [];

  // branchName/tests/children belong to the collapsed node.
  // Not up-to-date when there's unprocessed test objects.
  @computed get branchName() { return this.collapsedNode.branchName; }
  @computed get tests(): readonly ReadonlyTest[] { return this.collapsedNode.node.tests; }
  @computed get children(): readonly TestNode[] { return this.collapsedNode.node.immediateChildren; }
  @computed private get collapsedNode() {
    let branchName = this.immediateBranchName;

    // If the node has a single child, collapse it with its parent.
    let node: TestNode = this;
    while (node.immediateChildren.length === 1) {
      node = node.immediateChildren[0];
      branchName += node.immediateBranchName;
    }
    return {branchName, node};
  }

  static newRoot() { return new TestNode('', ''); }
  private constructor(prefix: string, branchName: string) {
    this.immediateBranchName = branchName;
    this.immediatePath = prefix + branchName;
  }

  /**
   * Contains all tests belong to this node and it's decendents.
   * Always up-to-date (i.e. contains tests that haven't been lazily evaluated into tree form yet).
   */
  @computed get allTests(): readonly ReadonlyTest[] { return this._allTests; }
  @observable.shallow private _allTests: ReadonlyTest[] = [];


  /**
   * Add a test to be processed.
   * The tree is not grown until this.processTests() is called.
   * @precondition prevTests.every((prev) => test.id.localeCompare(prev.id) >= 0)
   */
  addTest(test: ReadonlyTest) {
    // 'parent:child' has segments ['parent:', 'child'].
    // 'parent:child:' has segments ['parent:', 'child:', ''].
    // So testId ends with a non-alphanumerical character will have its own leaf
    // instead of being treated as a parent node when new testId with the previous
    // testId as its prefix is added.
    const idSegs = test.id.match(/\w*(\W|$)/g)!.reverse();
    // $ is matched twice when testId ends with \w, pop it.
    if (idSegs.length >= 2 && /\w$/.test(idSegs[idSegs.length - 2])) {
      idSegs.pop();
    }
    this.addTestWithIdSegs({test, idSegs});
  }

  private addTestWithIdSegs(test: {test: ReadonlyTest, idSegs: string[]}) {
    this.immediateUnbranchedTests.push(test);
    this._allTests.push(test.test);
  }

  /**
   * Convert all unprocessed tests in this node into branches of this node.
   * Recursively call this method on its desecedents if they are collapsed into this node.
   */
  @action
  branchUnbranchedTests() {
    for (const test of this.immediateUnbranchedTests) {
      this.branchTest(test);
    }
    if (this.immediateChildren.length === 1 && this.immediateUnbranchedTests.length !== 0) {
      this.immediateChildren[0].branchUnbranchedTests();
    }
    this.immediateUnbranchedTests = [];
  }

  private branchTest(test: {test: ReadonlyTest, idSegs: string[]}) {
    const nextSeg = test.idSegs.pop();
    // if there's no segments left, the test belongs to this node.
    if (!nextSeg) {
      this.immediateTests.push(test.test);
      return;
    }
    // TODO(weiweilin): we will only need to compare with the last child once all tests are sorted by testId.
    // crbug.com/1062117
    let child = this.immediateChildren.find((c) => c.immediateBranchName === nextSeg);
    if (child === undefined) {
      child = new TestNode(this.immediatePath, nextSeg);
      this.immediateChildren.push(child);
    }
    child.addTestWithIdSegs(test);
  }
}
