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

import { QueryTestVariantsRequest, TestVariant, TestVariantStatus, UISpecificService } from '../services/resultdb';
import { TestNode } from './test_node';


/**
 * Keeps the progress of the iterator and loads tests into the test node on
 * request.
 */
export class TestLoader {
  @computed get isLoading() { return !this.done && this.loadingReqCount !== 0; }
  @observable.ref private loadingReqCount = 0;

  @computed get done() { return this._done; }
  @observable.ref private _done = false;

  @observable.shallow readonly unexpectedTestVariants: TestVariant[] = [];
  @observable.shallow readonly flakyTestVariants: TestVariant[] = [];
  @observable.shallow readonly exoneratedTestVariants: TestVariant[] = [];
  @observable.shallow readonly expectedTestVariants: TestVariant[] = [];

  private nextBatch: Promise<IteratorResult<readonly TestVariant[]>>;

  constructor(
    readonly node: TestNode,
    private readonly testBatches: AsyncIterator<readonly TestVariant[]>,
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

    this.processTestVariants(next.value);
  }

  @action
  private processTestVariants(testVariants: readonly TestVariant[]) {
    for (const testVariant of testVariants) {
      switch (testVariant.status) {
        case TestVariantStatus.UNEXPECTED:
          this.unexpectedTestVariants.push(testVariant);
          break;
        case TestVariantStatus.FLAKY:
          this.flakyTestVariants.push(testVariant);
          break;
        case TestVariantStatus.EXONERATED:
          this.exoneratedTestVariants.push(testVariant);
          break;
        case TestVariantStatus.EXPECTED:
          this.expectedTestVariants.push(testVariant);
          break;
        default:
          break;
      }
      this.node.addTestId(testVariant.testId);
    }
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
