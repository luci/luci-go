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

import { QueryTestExonerationsRequest, QueryTestExonerationsResponse, QueryTestResultRequest, QueryTestResultsResponse, ResultDb,  TestExoneration, TestResult, Variant } from '../services/resultdb';
import { isTestResult, ReadonlyTest, ReadonlyVariant, TestNode, TestObject } from './test_node';


// Use a unexported class to artifically create file-scoped access modifier.
class TestVariant implements ReadonlyVariant {
  readonly results: TestResult[] = [];
  readonly exonerations: TestExoneration[] = [];
  constructor(readonly variant: Variant, readonly variantHash: string) {}
}


// Use a unexported class to artifically create file-scoped access modifier.
class Test implements ReadonlyTest {
  readonly variants: TestVariant[] = [];
  constructor(readonly id: string) {}

  private getOrCreateTestVariant(variant: Variant) {
    const variantHash = hashVariant(variant);
    let testVariant = this.variants.find((v) => v.variantHash === variantHash);
    if (testVariant === undefined) {
      testVariant = new TestVariant(variant, variantHash);
      this.variants.push(testVariant);
    }
    return testVariant;
  }

  addTestObject(testObject: TestObject) {
    const variant = this.getOrCreateTestVariant(testObject.variant || {def: {}});
    if (isTestResult(testObject)) {
      variant.results.push(testObject);
    } else {
      variant.exonerations.push(testObject);
    }
  }
}

function hashVariant(variant: Variant) {
  return Object.entries(variant.def)
    .map(([k, v]) => `${k},${v}`)
    .sort()
    .join('|');
}

/**
 * Stream test results from resultDb.
 */
export async function* streamTestResults(req: QueryTestResultRequest, resultDb: ResultDb) {
  let pageToken = req.pageToken;
  do {
    const res: QueryTestResultsResponse = await resultDb.queryTestResults({
      ...req,
      pageToken,
    });
    pageToken = res.nextPageToken;
    yield* res.testResults;
  } while (pageToken);
}

/**
 * Stream test exoneration from resultDb.
 */
export async function* streamTestExonerations(req: QueryTestExonerationsRequest, resultDb: ResultDb) {
  let pageToken = req.pageToken;
  do {
    const res: QueryTestExonerationsResponse = await resultDb.queryTestExonerations({
      ...req,
      pageToken,
    });
    pageToken = res.nextPageToken;
    yield* res.testExonerations;
  } while (pageToken);
}

/**
 * Combines results generator and exoneration generator.
 * Yields TestObject sorted by testId.
 * When a result and an exoneration have the same ID, result is yielded first.
 * @param results must be sorted by testId.
 * @param exonerations must be sorted by testId.
 */
export async function* streamTestObjects(results: AsyncGenerator<TestResult>, exonerations: AsyncGenerator<TestExoneration>) {
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
 * Groups test objects into Test (i.e. group by testId, then by variant)
 * @param testObjs: must be sorted by testId.
 */
export async function* streamTests(testObjs: AsyncIterator<TestObject>): AsyncIterableIterator<ReadonlyTest> {
  let test: Test | undefined;
  let nextTestObject = await testObjs.next();
  while (!nextTestObject.done) {
    if (nextTestObject.value.testId === test?.id) {
      test.addTestObject(nextTestObject.value);
    } else {
      if (test !== undefined) {
        yield test;
      }
      test = new Test(nextTestObject.value.testId);
      test.addTestObject(nextTestObject.value);
    }
    nextTestObject = await testObjs.next();
  }
  if (test !== undefined) {
    yield test;
  }
}

/**
 * Keeps the progress of the iterator and load tests into a test node on request.
 */
export class TestLoader {
  constructor(readonly node: TestNode, private testIter: AsyncIterator<ReadonlyTest>) {}

  /**
   * load more tests from the iterator to the node.
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
