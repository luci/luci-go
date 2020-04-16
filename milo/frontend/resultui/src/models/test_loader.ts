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

import { computed, observable } from 'mobx';

import { QueryTestExonerationsRequest, QueryTestResultRequest, ResultDb,  TestExoneration, TestResult, Variant } from '../services/resultdb';
import { isTestResult, ReadonlyTest, ReadonlyVariant, TestNode, TestResultOrExoneration, VariantStatus } from './test_node';


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

  /**
   * @param testIter the tests should be sorted by id.
   */
  constructor(
    readonly node: TestNode,
    private readonly testIter: AsyncIterator<ReadonlyTest>,
  ) {}

  private loadPromise = Promise.resolve();

  /**
   * Loads more tests from the iterator to the node.
   */
  loadMore(limit = 100) {
    if (this.done) {
      return this.loadPromise;
    }
    this.loadingReqCount++;
    // TODO(weiweilin): better error handling.
    this.loadPromise = this.loadPromise.then(() => this.loadMoreInternal(limit));
    return this.loadPromise.then(() => this.loadingReqCount--);
  }

  /**
   * Loads more tests from the iterator to the node.
   *
   * @precondition there should not exist a running instance of
   * this.loadMoreInternal
   */
  private async loadMoreInternal(limit: number) {
    while (limit > 0) {
      const {value, done} = await this.testIter.next();
      if (done) {
        this._done = true;
        this.node.finalizeLoading();
        break;
      }
      this.node.addTest(value);
      limit--;
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
  // Use an key-index map for O(1) lookup.
  private readonly variantKeyIndexMap: {[key: string]: number | undefined} = {};

  // Use an array instead of a dictionary so
  //   1. the order is stable when new variants are added.
  //   2. we can keep variantKey as an implementation detail.
  @observable.shallow readonly variants: TestVariant[] = [];
  constructor(readonly id: string) {}

  private getOrCreateTestVariant(variant: Variant) {
    const variantKey = keyForVariant(variant);
    let variantIndex = this.variantKeyIndexMap[variantKey];
    if (variantIndex === undefined) {
      variantIndex = this.variants.length;
      this.variants.push(new TestVariant(variant, variantKey));
      this.variantKeyIndexMap[variantKey] = variantIndex;
    }
    return this.variants[variantIndex];
  }

  addTestResultOrExoneration(testResultOrExoneration: TestResultOrExoneration) {
    const variant = this.getOrCreateTestVariant(testResultOrExoneration.variant || {def: {}});
    if (isTestResult(testResultOrExoneration)) {
      variant.results.push(testResultOrExoneration);
    } else {
      variant.exonerations.push(testResultOrExoneration);
    }
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
export async function* streamTestResults(req: QueryTestResultRequest, resultDb: ResultDb) {
  let pageToken = req.pageToken;
  do {
    const res = await resultDb.queryTestResults({...req, pageToken});
    pageToken = res.nextPageToken;
    yield* res.testResults;
  } while (pageToken);
}

/**
 * Streams test exonerations from resultDb.
 */
export async function* streamTestExonerations(req: QueryTestExonerationsRequest, resultDb: ResultDb) {
  let pageToken = req.pageToken;
  do {
    const res = await resultDb.queryTestExonerations({...req, pageToken});
    pageToken = res.nextPageToken;
    yield* res.testExonerations;
  } while (pageToken);
}

/**
 * Combines results generator and exoneration generator.
 * Yields TestResultOrExoneration with variant key sorted by testId.
 * When a result and an exoneration have the same ID, the result is yielded
 * first.
 * @param results must be sorted by testId.
 * @param exonerations must be sorted by testId.
 */
async function* streamTestResultOrExonerations(results: AsyncIterableIterator<TestResult>, exonerations: AsyncIterableIterator<TestExoneration>) {
  let [resultNext, exonerationNext] = await Promise.all([results.next(), exonerations.next()]);

  while (!resultNext.done && !exonerationNext.done) {
    if (resultNext.value.testId.localeCompare(exonerationNext.value.testId) <= 0) {
      yield resultNext.value;
      resultNext = await results.next();
    } else {
      yield exonerationNext.value;
      exonerationNext = await exonerations.next();
    }
  }
  if (!resultNext.done) {
    yield resultNext.value;
    yield* results;
  }
  if (!exonerationNext.done) {
    yield exonerationNext.value;
    yield* exonerations;
  }
}

/**
 * Groups test results and exonerations into Test objects.
 * @param results must be sorted by testId.
 * @param exonerations must be sorted by testId.
 */
export async function* streamTests(results: AsyncIterableIterator<TestResult>, exonerations: AsyncIterableIterator<TestExoneration>): AsyncIterableIterator<ReadonlyTest> {
  const testResultOrExonerations = streamTestResultOrExonerations(results, exonerations);
  let test: Test | undefined;
  for await (const nextResultOrExoneration of testResultOrExonerations) {
    if (nextResultOrExoneration.testId === test?.id) {
      test.addTestResultOrExoneration(nextResultOrExoneration);
    } else {
      if (test !== undefined) {
        yield test;
      }
      test = new Test(nextResultOrExoneration.testId);
      test.addTestResultOrExoneration(nextResultOrExoneration);
    }
  }
  if (test !== undefined) {
    yield test;
  }
}
