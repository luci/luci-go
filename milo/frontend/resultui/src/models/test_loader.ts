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

import { QueryTestVariantsRequest, TestVariant, UISpecificService } from '../services/resultdb';
import { ReadonlyTest, TestNode } from './test_node';


/**
 * Keeps the progress of the iterator and loads tests into the test node on
 * request.
 * Instead of taking QueryTestResultRequest/QueryTestExonerationRequest,
 * TestLoader takes an AsyncIterator<ReadonlyTest[]>, which can be constructed
 * by combining stream... functions in this file.
 * This enables the caller to filter the result, exoneration, or tests at
 * different levels.
 */
export class TestLoader {
  @computed get isLoading() { return !this.done && this.loadingReqCount !== 0; }
  @observable.ref private loadingReqCount = 0;

  @computed get done() { return this._done; }
  @observable.ref private _done = false;

  private nextBatch: Promise<IteratorResult<readonly ReadonlyTest[]>>;

  constructor(
    readonly node: TestNode,
    private readonly testBatches: AsyncIterator<readonly ReadonlyTest[]>,
  ) {
    this.nextBatch = this.testBatches.next();
    this.nextBatch.then((v) => this._done = Boolean(v.done));
  }

  private loadPromise = Promise.resolve();

  /**
   * Loads the next batch of tests from the iterator to the node.
   */
  loadNextPage() {
    if (this.done) {
      return this.loadPromise;
    }
    this.loadingReqCount++;
    this.loadPromise = this.loadPromise.then(() => this.loadNextPageInternal());
    return this.loadPromise.then(() => this.loadingReqCount--);
  }

  /**
   * Loads the next batch of tests from the iterator to the node.
   *
   * @precondition there should not exist a running instance of
   * this.loadMoreInternal
   */
  private async loadNextPageInternal() {
    const next = await this.nextBatch;
    if (next.done) {
      return;
    }

    // Prefetch the next batch so the UI is more responsive and we can mark the
    // set this._done to true when there's no more batches.
    this.nextBatch = this.testBatches.next();
    this.nextBatch.then((v) => this._done = Boolean(v.done));

    this.processTests(next.value);
  }

  @action
  private processTests(tests: readonly ReadonlyTest[]) {
    for (const test of tests) {
      this.node.addTest(test);
    }
  }
}

/**
 * Test with methods to help grouping test results or exonerations by variants.
 */
// Use an unexported class that implements an exported interface to artificially
// create file-scoped access modifier.
class Test implements ReadonlyTest {
  // Use a map for O(1) lookup.
  private readonly variantMap = observable.map(new Map<string, TestVariant>(), {deep: false});

  @computed get variants(): readonly TestVariant[] {
    return [...this.variantMap.entries()]
      .sort(([key1], [key2]) => key1.localeCompare(key2))
      .map(([_k, v]) => v);
  }

  constructor(readonly id: string) {}

  addTestVariant(testVariant: TestVariant) {
    this.variantMap.set(testVariant.variantHash, testVariant);
  }
}

/**
 * Groups test results and exonerations into variant objects.
 * The number of exonerations is assumed to be low. All exonerations are loaded
 * in the first batch.
 * Yielded variants could be modified when new results are processed.
 * The order of variants is not guaranteed.
 */
export async function* streamVariantBatches(req: QueryTestVariantsRequest, uiSpecificService: UISpecificService): AsyncIterableIterator<readonly TestVariant[]> {
  let pageToken = req.pageToken;

  do {
    const res = await uiSpecificService.queryTestVariants({...req, pageToken});
    pageToken = res.nextPageToken;
    yield res.testVariants || [];
  } while (pageToken);
}

/**
 * Groups test variants into tests.
 * Yielded tests could be modified when new variants are processed.
 * The order of tests is not guaranteed.
 */
export async function* streamTestBatches(variantBatches: AsyncIterable<readonly TestVariant[]>): AsyncIterableIterator<readonly ReadonlyTest[]> {
  const testMap = new Map<string, Test>();

  for await (const variantBatch of variantBatches) {
    const newTests = [] as Test[];
    for (const variant of variantBatch) {
      let test = testMap.get(variant.testId);
      if (!test) {
        test = new Test(variant.testId);
        testMap.set(test.id, test);
        newTests.push(test);
      }
      test.addTestVariant(variant);
    }
    yield newTests;
  }
}
