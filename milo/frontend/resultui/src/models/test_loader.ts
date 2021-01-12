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
 * The stage of the next test variant. The stage can be
 * 1. LoadingXXX: the status of the next test variant will be no worse than XXX.
 * 2. Done: all test variants have been loaded.
 */
export const enum LoadingStage {
  LoadingUnexpected = 0,
  LoadingFlaky = 1,
  LoadingExonerated = 2,
  LoadingExpected = 3,
  Done = 4,
}

/**
 * Keeps the progress of the iterator and loads tests into the test node on
 * request.
 */
export class TestLoader {
  @computed get isLoading() { return this.stage !== LoadingStage.Done && this.loadingReqCount !== 0; }
  @observable.ref private loadingReqCount = 0;

  /**
   * The queryTestVariants RPC sorted the variant by status. We can use this to
   * tell the possible status of the next test variants and therefore avoid
   * unnecessary loading.
   */
  @computed get stage() { return this._stage; }
  @observable.ref private _stage = LoadingStage.LoadingUnexpected;

  @observable.shallow readonly unexpectedTestVariants: TestVariant[] = [];
  @observable.shallow readonly flakyTestVariants: TestVariant[] = [];
  @observable.shallow readonly exoneratedTestVariants: TestVariant[] = [];
  @observable.shallow readonly expectedTestVariants: TestVariant[] = [];

  // undefined means the end has been reached.
  // empty string is the token for the first page.
  private nextPageToken: string | undefined = '';

  constructor(
    readonly node: TestNode,
    private readonly req: QueryTestVariantsRequest,
    private readonly uiSpecificService: UISpecificService,
  ) {}

  private loadPromise = Promise.resolve();

  /**
   * Loads the next batch of tests from the iterator to the node.
   */
  loadNextPage() {
    if (this.stage === LoadingStage.Done) {
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
    if (this.nextPageToken === undefined) {
      return;
    }

    const res = await this.uiSpecificService
      .queryTestVariants({...this.req, pageToken: this.nextPageToken});
    this.nextPageToken = res.nextPageToken;

    this.processTestVariants(res.testVariants || []);
    if (this.nextPageToken === undefined) {
      this._stage = LoadingStage.Done;
      return;
    }
    if (res.testVariants.length < (this.req.pageSize || 1000)) {
      // When the service returns an incomplete page and nextPageToken is not
      // undefined, the following pages must be expected test variants.
      this._stage = LoadingStage.LoadingExpected;
      return;
    }
  }

  @action
  private processTestVariants(testVariants: readonly TestVariant[]) {
    for (const testVariant of testVariants) {
      switch (testVariant.status) {
        case TestVariantStatus.UNEXPECTED:
          this._stage = LoadingStage.LoadingUnexpected;
          this.unexpectedTestVariants.push(testVariant);
          break;
        case TestVariantStatus.FLAKY:
          this._stage = LoadingStage.LoadingFlaky;
          this.flakyTestVariants.push(testVariant);
          break;
        case TestVariantStatus.EXONERATED:
          this._stage = LoadingStage.LoadingExonerated;
          this.exoneratedTestVariants.push(testVariant);
          break;
        case TestVariantStatus.EXPECTED:
          this._stage = LoadingStage.LoadingExpected;
          this.expectedTestVariants.push(testVariant);
          break;
        default:
          break;
      }
      this.node.addTestId(testVariant.testId);
    }
  }
}
