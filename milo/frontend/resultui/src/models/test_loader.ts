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
 * @overview This file contains functions/classes that helps loading test
 * results and exonerations from resultDb to a TestNode.
 */

import { asyncIterableMap } from '../libs/iterable_map';
import { QueryTestExonerationsRequest, QueryTestResultRequest, ResultDb,  TestExoneration, TestResult, Variant } from '../services/resultdb';
import { isTestResult, ReadonlyTest, ReadonlyVariant, TestNode, TestResultOrExoneration } from './test_node';


/**
 * Keeps the progress of the iterators, group test results and exonerations into
 * tests and loads tests into the test node on request.
 * Instead of taking QueryTestResultRequest/QueryTestExonerationRequest,
 * TestLoader takes an  AsyncIterator<ReadonlyTest>, which can be constructed
 * by combining stream... functions in this file.
 * This enables the caller to filter the result, exoneration, or tests at
 * different levels.
 */
export class TestLoader {
  /**
   * @precondition the items yield by resultIter and exonerationIter must be
   * sorted by testId, then by variant.
   */
  constructor(
    readonly node: TestNode,
    private readonly testIter: AsyncIterator<ReadonlyTest>,
  ) {}

  /**
   * Loads more tests from the iterator to the node.
   */
  async loadMore(limit = 100) {
    while (limit > 0) {
      const next = await this.testIter.next();
      if (next.done) { return; }
      this.node.addTest(next.value);
      limit -= 1;
    }
  }
}


/**
 * Test variant with variantHash to help comparing variant faster.
 */
// Use an unexported class that implements an exported interface to artificially
// create file-scoped access modifier.
class TestVariant implements ReadonlyVariant {
  readonly results: TestResult[] = [];
  readonly exonerations: TestExoneration[] = [];
  constructor(readonly variant: Variant, readonly variantKey: string) {}
}

/**
 * Test with methods to help grouping test results or exonerations by variants.
 */
// Use an unexported class that implements an exported interface to artificially
// create file-scoped access modifier.
class Test implements ReadonlyTest {
  readonly variants: TestVariant[] = [];
  constructor(readonly id: string) {}

  private getOrCreateTestVariant(variant: Variant, variantKey: string) {
    let testVariant = this.variants.last;
    if (testVariant?.variantKey !== variantKey) {
      testVariant = new TestVariant(variant, variantKey);
      this.variants.push(testVariant);
    }
    return testVariant;
  }

  /**
   * @precondition testResultOrExoneration should have a variant key no less
   * than any of the keys of existing testResultOrExonerations
   */
  addTestResultOrExoneration(testResultOrExoneration: TestResultOrExoneration, variantKey: string) {
    const variant = this.getOrCreateTestVariant(testResultOrExoneration.variant || {def: {}}, variantKey);
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
 * Streams test exoneration from resultDb.
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
 * Yields TestResultOrExoneration with variant key sorted by testId, then by
 * variant key.
 * When a result and an exoneration have the same ID and variant,
 * result is yielded first.
 * @param results must be sorted by testId, then by variant.
 * @param exonerations must be sorted by testId, then by variant.
 */
async function* streamTestResultOrExonerations(results: AsyncIterable<TestResult>, exonerations: AsyncIterable<TestExoneration>): AsyncGenerator<[TestResultOrExoneration, string]> {
  const resultWithVariantIter = asyncIterableMap<TestResult, [TestResult, string]>(
    results,
    (result) => [result, keyForVariant(result.variant || {def: {}})],
  );
  const exonerationWithVariantIter = asyncIterableMap<TestExoneration, [TestExoneration, string]>(
    exonerations,
    (exoneration) => [exoneration, keyForVariant(exoneration.variant || {def: {}})],
  );
  let [resultNext, exonerationNext] = await Promise.all([resultWithVariantIter.next(), exonerationWithVariantIter.next()]);

  while (!resultNext.done && !exonerationNext.done) {
    const testIdLocalCompareResult = resultNext.value[0].testId.localeCompare(exonerationNext.value[0].testId);
    if (testIdLocalCompareResult < 0 || testIdLocalCompareResult === 0 && resultNext.value[1].localeCompare(exonerationNext.value[1]) <= 0) {
      yield resultNext.value;
      resultNext = await resultWithVariantIter.next();
    } else {
      yield exonerationNext.value;
      exonerationNext = await exonerationWithVariantIter.next();
    }
  }
  if (!resultNext.done) {
    yield resultNext.value;
    yield* resultWithVariantIter;
  }
  if (!exonerationNext.done) {
    yield exonerationNext.value;
    yield* exonerationWithVariantIter;
  }
}

/**
 * Groups test results and exonerations into Test
 * @precondition results and exonerations must be sorted by testId then by
 * variant.
 */
export async function* streamTests(results: AsyncIterable<TestResult>, exonerations: AsyncIterable<TestExoneration>): AsyncIterableIterator<ReadonlyTest> {
  const testResultOrExonerations = streamTestResultOrExonerations(results, exonerations);
  let test: Test | undefined;
  for await (const nextResultOrExoneration of testResultOrExonerations) {
    if (nextResultOrExoneration[0].testId === test?.id) {
      test.addTestResultOrExoneration(...nextResultOrExoneration);
    } else {
      if (test !== undefined) {
        yield test;
      }
      test = new Test(nextResultOrExoneration[0].testId);
      test.addTestResultOrExoneration(...nextResultOrExoneration);
    }
  }
  if (test !== undefined) {
    yield test;
  }
}
