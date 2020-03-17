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

import deepEqual from 'deep-equal';
import { action, computed, observable } from 'mobx';

import { QueryTestExonerationsResponse, QueryTestResultsResponse, ResultDb, TestExoneration, TestResult, Variant } from '../services/resultdb';


export type TestOutput = TestResult | TestExoneration;

export function isTestResult(testOutput: TestOutput): testOutput is TestResult {
    return (testOutput as TestResult).resultId !== undefined;
}


// tslint:disable-next-line: interface-name
export interface ITest {
  readonly id: string;
  readonly variants: ReadonlyArray<ITestVariant>;
}

class Test implements ITest {
  readonly variants: TestVariant[] = [];
  constructor(readonly id: string) {}

  addOutput(output: TestOutput) {
    let variant = this.variants.find((v) => deepEqual(v.variant, output.variant));
    if (variant === undefined) {
      variant = new TestVariant(output.variant!);
      this.variants.push(variant);
    }
    if (isTestResult(output)) {
      variant.results.push(output);
    } else {
      variant.exonerations.push(output);
    }
  }
}


// tslint:disable-next-line: interface-name
export interface ITestVariant {
  readonly variant: Variant;
  readonly results: ReadonlyArray<TestResult>;
  readonly exonerations: ReadonlyArray<TestExoneration>;
}

class TestVariant implements ITestVariant {
  readonly results: TestResult[] = [];
  readonly exonerations: TestExoneration[] = [];
  constructor(readonly variant: Variant) {}
}

/**
 * Store test in a tree like sturcture.
 * Node with no siblings are collapsed into their parents.
 */
export class TestNode {
  private readonly directBranchName: string;
  readonly directPath: string;
  @observable.shallow directUnprocessedTests: Array<{test: ITest, idSegs: string[]}> = [];

  // tests/children directly belong to this node.
  // Not up-to-date when there's unprocessed output.
  @observable.shallow private directTests: ITest[] = [];
  @observable.shallow private directChildren: TestNode[] = [];

  // branchName/tests/children belong to the collapsed node.
  // Not up-to-date when there's unprocessed output.
  @computed get branchName() { return this.collapsedNode.branchName; }
  @computed get tests(): readonly ITest[] { return this.collapsedNode.node.tests; }
  @computed get children(): readonly TestNode[] { return this.collapsedNode.node.directChildren; }
  @computed private get collapsedNode() {
    let branchName = this.directBranchName;

    // If the node has a single child, collapse it with its parent.
    let node: TestNode = this;
    while (node.directChildren.length === 1) {
      node = node.directChildren[0];
      branchName += node.directBranchName;
    }
    return {branchName, node};
  }

  static newRoot() { return new TestNode('', ''); }
  private constructor(prefix: string, branchName: string) {
    this.directBranchName = branchName;
    this.directPath = prefix + branchName;
  }

  /**
   * All tests belong to this node and it's decendents.
   * Always up-to-date.
   */
  @computed get allTests(): readonly ITest[] { return this._allTests; }
  @observable.shallow private _allTests: ITest[] = [];


  /**
   * Add a test to be processed.
   * The tree is not grown until this.processTests() is called.
   * @precondition prevTests.every((prev) => test.id.localeCompare(prev.id) >= 0)
   */
  addTest(test: ITest) {
    // 'parent:child' has segments ['parent:', 'child'].
    // 'parent:child:' has segments ['parent:', 'child:', ''].
    // So testId ends with a non-alphanumerical charater will have its own leaf
    // instead of being treated as a parent node when new testId with the previous
    // testId as its prefix is added.
    const idSegs = test.id.match(/\w*(\W|$)/g)!.reverse();
    // $ is matched twice when testId ends with \w, pop it.
    if (idSegs.length >= 2 && /\w$/.test(idSegs[idSegs.length - 2])) {
      idSegs.pop();
    }
    this.addTestWithIdSegs({test, idSegs});
  }

  private addTestWithIdSegs(test: {test: ITest, idSegs: string[]}) {
    this.directUnprocessedTests.push(test);
    this._allTests.push(test.test);
  }

  /**
   * Process all unprocessed tests.
   */
  @action
  processTests() {
    for (const test of this.directUnprocessedTests) {
      this.processTest(test);
    }
    if (this.directChildren.length === 1 && this.directUnprocessedTests.length !== 0) {
      this.directChildren[0].processTests();
    }
    this.directUnprocessedTests = [];
  }

  private processTest(test: {test: ITest, idSegs: string[]}) {
    const nextSeg = test.idSegs.pop();
    if (!nextSeg) {
      this.directTests.push(test.test);
      return;
    }
    // if (nextSeg !== this.directChildren.last?.directBranchName) {
    //   this.directChildren.push(new TestNode(this.directPath, nextSeg));
    // }
    // this.directChildren.last!.addTestWithIdSegs(test);

    // TODO(weiweilin): change the implementation to above once we can ensure all tests are sorted by testId.
    let branch = this.directChildren.find((child) => child.directBranchName === nextSeg);
    if (branch === undefined) {
      branch = new TestNode(this.directPath, nextSeg);
      this.directChildren.push(branch);
    }
    branch.addTestWithIdSegs(test);
  }
}

export class TestLoader {
  readonly root: TestNode;
  private readonly testOutputIter: AsyncIterableIterator<TestOutput>;

  constructor(readonly invocationName: string, readonly resultDb: ResultDb) {
    this.root = TestNode.newRoot();
    const resultIter = (async function*() {
      let pageToken: string | undefined = '';
      do {
        const res: QueryTestResultsResponse = await resultDb.queryTestResults({
          invocations: [invocationName],
          pageToken,
        });
        pageToken = res.nextPageToken;
        for (const result of res.testResults) {
          yield result;
        }
      } while (pageToken !== undefined);
    })();

    const exonerationIter = (async function*() {
      let pageToken: string | undefined = '';
      do {
        const res: QueryTestExonerationsResponse = await resultDb.queryTestExonerations({
          invocations: [invocationName],
          pageToken,
        });
        pageToken = res.nextPageToken;
        for (const exoneration of res.testExonerations) {
          yield exoneration;
        }
      } while (pageToken !== undefined);
    })();

    this.testOutputIter = (async function*() {
      let [resultNext, exonerationNext] = await Promise.all([resultIter.next(), exonerationIter.next()]);
      while (!resultNext.done && !exonerationNext.done) {
        if (resultNext.value.testId.localeCompare(exonerationNext.value.testId) <= 0) {
          yield resultNext.value;
          resultNext = await resultIter.next();
        } else {
          yield exonerationNext.value;
          exonerationNext = await exonerationIter.next();
        }
      }
      while (!resultNext.done) {
        yield resultNext.value;
        resultNext = await resultIter.next();
      }
      while (!exonerationNext.done) {
        yield exonerationNext.value;
        exonerationNext = await exonerationIter.next();
      }
    })();
  }

  async loadMore(count = 1000) {
    let test: Test | undefined;
    for await (const output of this.testOutputIter) {
      if (output.testId === test?.id) {
        test.addOutput(output);
      } else {
        if (test !== undefined) {
          this.root.addTest(test);
          count -= 1;
          if (count === 0) {
            break;
          }
        }
        test = new Test(output.testId);
        test.addOutput(output);
      }
    }
    if (test !== undefined) {
      this.root.addTest(test);
    }
  }
}
