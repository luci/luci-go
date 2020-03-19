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
 * TestResult or TestExoneration
 */
export type TestResultOrExoneration = TestResult | TestExoneration;

/**
 * Tests if a TestResultOrExoneration is a test result.
 */
export function isTestResult(testResultOrExoneration: TestResultOrExoneration): testResultOrExoneration is TestResult {
  return (testResultOrExoneration as TestResult).resultId !== undefined;
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
 */
export class TestNode {
  // The path leads to this node, including the name of this node.
  readonly path: string;

  @observable.shallow private tests: ReadonlyTest[] = [];
  @observable.shallow private children: TestNode[] = [];

  // elided... means the property belongs to the elided node (descendants with no siblings are elided into this node).
  @computed get elidedName() { return this.elidedNode.name; }
  @computed get elidedTests(): readonly ReadonlyTest[] { return this.elidedNode.node.elidedTests; }
  @computed get elidedChildren(): readonly TestNode[] { return this.elidedNode.node.children; }
  @computed private get elidedNode() {
    let name = this.name;

    // If the node has a single child, elide it into its parent.
    let node: TestNode = this;
    while (node.children.length === 1) {
      node = node.children[0];
      name += node.name;
    }
    return {name, node};
  }

  static newRoot() { return new TestNode('', ''); }
  private constructor(prefix: string, private readonly name: string) {
    this.path = prefix + name;
  }

  /**
   * Contains all tests belonging to this node and its descendants.
   * Always up-to-date (i.e. contains tests that haven't been lazily evaluated into tree form yet).
   */
  @computed get allTests(): readonly ReadonlyTest[] { return this._allTests; }
  @observable.shallow private _allTests: ReadonlyTest[] = [];

  /**
   * Adds a test to be branched.
   * @precondition prevTests.every((prev) => test.id.localeCompare(prev.id) >= 0)
   */
  addTest(test: ReadonlyTest) {
    // Use /\w*(\W+|$)/g instead of /\w+(\W+|$)/g so testIds ending with a non-alphanumerical character will get their own leaf.
    // This ensures only leaf nodes can have directly associated tests.
    // Without this, nodes may be incorrectly elided when there's a testId that ends with /\W/.
    // For example, when we add 'parent:' and 'parent:child' to the tree, 'child' will be incorrectly elided into 'parent:',
    // even though 'parent:' contains two different testIds.
    const idSegs = test.id.match(/\w*(\W+|$)/g)!;
    // testId that ends with /\w/ will have an unwanted empty string segment, pop it.
    // For example, 'a:b'.match(/\w*(\W|$)/g) returns ['a', 'b', ''], but the segments should be ['a', 'b'].
    if (idSegs.length >= 2 && /\w$/.test(idSegs[idSegs.length - 2])) {
      idSegs.pop();
    }
    idSegs.reverse();
    this.addTestWithIdSegs(test, idSegs);
  }

  private addTestWithIdSegs(test: ReadonlyTest, idSegs: string[]) {
    this.branchTest(test, idSegs);
    this._allTests.push(test);
  }

  /**
   * Takes a test and adds it to the appropriate place in the tree, creating new nodes as necessary.
   */
  private branchTest(test: ReadonlyTest, idSegs: string[]) {
    const nextSeg = idSegs.pop();
    // If there are no segments left, the test belongs to this node.
    if (nextSeg === undefined) {
      this.tests.push(test);
      return;
    }
    // TODO(weiweilin): we will only need to compare with the last child once all tests are sorted by testId.
    // crbug.com/1062117
    let child = this.children.find((c) => c.name === nextSeg);
    if (child === undefined) {
      child = new TestNode(this.path, nextSeg);
      this.children.push(child);
    }
    child.addTestWithIdSegs(test, idSegs);
  }
}
