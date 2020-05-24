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

/**
 * @fileoverview This file contains functions/classes that helps loading test
 * results and exonerations from resultDb to a TestNode.
 */

import { action, computed, observable } from 'mobx';

import { QueryTestExonerationsRequest, QueryTestResultRequest, ResultDb,  TestExoneration, TestResult, Variant } from '../services/resultdb';
import { ReadonlyTest, ReadonlyVariant, TestNode, VariantStatus } from './test_node';


/**
 * Keeps the progress of the iterators, group test results and exonerations into
 * tests and loads tests into the test node on request.
 * Instead of taking QueryTestResultRequest/QueryTestExonerationRequest,
 * TestLoader takes an AsyncIterator<ReadonlyTest>, which can be constructed
 * by combining stream... functions in this file.
 * This enables the caller to filter the result, exoneration, or tests at
 * different levels.
 */
export class TestLoader {
  @computed get isLoading() { return !this.done && this.loadingReqCount !== 0; }
  @observable.ref private loadingReqCount = 0;

  @computed get done() { return this._done; }
  @observable.ref private _done = false;

  constructor(
    readonly node: TestNode,
    private readonly testBatches: AsyncIterator<[ReadonlyTest[], boolean]>,
  ) {
  }

  private loadPromise = Promise.resolve();

  /**
   * Loads more tests from the iterator to the node.
   */
  loadNextPage() {
    if (this.done) {
      return this.loadPromise;
    }
    this.loadingReqCount++;
    // TODO(weiweilin): better error handling.
    this.loadPromise = this.loadPromise.then(() => this.loadMoreInternal());
    return this.loadPromise.then(() => this.loadingReqCount--);
  }

  /**
   * Loads more tests from the iterator to the node.
   *
   * @precondition there should not exist a running instance of
   * this.loadMoreInternal
   */
  private async loadMoreInternal() {
    const next = await this.testBatches.next();
    if (next.done) {
      this._done = true;
      return;
    }
    this.processTests(next.value[0]);

    if (next.value[1]) {
      this._done = true;
    }
  }

  @action
  private processTests(tests: ReadonlyTest[]) {
    for (const test of tests) {
      this.node.addTest(test);
    }
  }
}


/**
 * Contains a list of results and exonerations for a given test variant.
 * Includes a string key for faster comparison of variants.
 */
// Use an unexported class that implements an exported interface to artificially
// create file-scoped access modifier.
class TestVariant implements ReadonlyVariant {
  @computed get status() {
    if (this.results.length === 0) {
      return VariantStatus.Exonerated;
    }
    const firstExpected = Boolean(this.results[0].expected);
    for (const result of this.results) {
      if (Boolean(result.expected) !== firstExpected) {
        return VariantStatus.Flaky;
      }
    }
    return firstExpected ? VariantStatus.Expected : VariantStatus.Unexpected;
  }
  @observable.shallow readonly results: TestResult[] = [];
  @observable.shallow readonly exonerations: TestExoneration[] = [];
  constructor(readonly variant: Variant, readonly variantKey: string) {}
}

/**
 * Test with methods to help grouping test results or exonerations by variants.
 */
// Use an unexported class that implements an exported interface to artificially
// create file-scoped access modifier.
class Test implements ReadonlyTest {
  // Use a map for O(1) lookup.
  @observable private readonly variantMap = new Map<string, TestVariant>();

  @computed get variants(): readonly TestVariant[] {
    return [...this.variantMap.entries()]
      .sort(([key1], [key2]) => key1.localeCompare(key2))
      .map(([_k, v]) => v);
  }

  constructor(readonly id: string) {}

  private getOrCreateTestVariant(variant: Variant) {
    const variantKey = keyForVariant(variant);
    let testVariant = this.variantMap.get(variantKey);
    if (testVariant === undefined) {
      testVariant = new TestVariant(variant, variantKey);
      this.variantMap.set(variantKey, testVariant);
    }
    return testVariant;
  }

  addResult(result: TestResult) {
    const variant = this.getOrCreateTestVariant(result.variant || {def: {}});
    variant.results.push(result);
  }

  addExoneration(exoneration: TestExoneration) {
    const variant = this.getOrCreateTestVariant(exoneration.variant || {def: {}});
    variant.exonerations.push(exoneration);
  }
}

/**
 * Computes a unique string key of the variant.
 */
function keyForVariant(variant: Variant) {
  if (!variant) {
    return '';
  }
  return Object.entries(variant.def)
    .map((kv) => kv.join(':'))
    .sort()
    .join('|');
}

/**
 * Streams test results from resultDb.
 */
export async function* streamTestResultBatches(req: QueryTestResultRequest, resultDb: ResultDb): AsyncIterableIterator<[TestResult[], boolean]> {
  let pageToken = req.pageToken;
  do {
    const res = await resultDb.queryTestResults({...req, pageToken});
    pageToken = res.nextPageToken;
    yield [res.testResults, !pageToken];
  } while (pageToken);
}

/**
 * Streams test exonerations from resultDb.
 */
export async function* streamTestExonerationBatches(req: QueryTestExonerationsRequest, resultDb: ResultDb): AsyncIterableIterator<[TestExoneration[], boolean]> {
  let pageToken = req.pageToken;
  do {
    const res = await resultDb.queryTestExonerations({...req, pageToken});
    pageToken = res.nextPageToken;
    yield [res.testExonerations, !pageToken];
  } while (pageToken);
}

/**
 * Groups test results and exonerations into Test objects.
 * Yielded tests could be modified when new results and exonerations are
 * fetched.
 * Exonerations are loaded before results and the number of exonerations is
 * expected to be small.
 */
export async function* streamTestBatches(resultBatches: AsyncIterator<[TestResult[], boolean]>, exonerationBatches: AsyncIterator<[TestExoneration[], boolean]>): AsyncIterableIterator<[ReadonlyTest[], boolean]> {
  const testMap = new Map<string, Test>();
  let [nextResultBatch, nextExonerationBatch] = await Promise.all([
    resultBatches.next().then((v) => v.done ? [[], true] as [TestResult[], boolean] : v.value),
    exonerationBatches.next().then((v) => v.done ? [[], true] as [TestExoneration[], boolean] : v.value),
  ]);

  while (true) {
    const newTests = [] as ReadonlyTest[];
    for (const result of nextResultBatch[0]) {
      let test = testMap.get(result.testId);
      if (!test) {
        test = new Test(result.testId);
        testMap.set(result.testId, test);
        newTests.push(test);
      }
      test.addResult(result);
    }

    for (const exoneration of nextExonerationBatch[0]) {
      let test = testMap.get(exoneration.testId);
      if (!test) {
        test = new Test(exoneration.testId);
        testMap.set(exoneration.testId, test);
        newTests.push(test);
      }
      test.addExoneration(exoneration);
    }

    const done = nextResultBatch[1] && nextExonerationBatch[1];
    yield [newTests, done];
    if (done) {
      break;
    }

    [nextResultBatch, nextExonerationBatch] = await Promise.all([
      resultBatches.next().then((v) => v.done ? [[], true] as [TestResult[], boolean] : v.value),
      exonerationBatches.next().then((v) => v.done ? [[], true] as [TestExoneration[], boolean] : v.value),
    ]);
  }
}
