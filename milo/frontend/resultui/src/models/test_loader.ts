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

import { groupBy } from 'lodash-es';
import { action, computed, makeObservable, observable } from 'mobx';

import { InnerTag, TAG_SOURCE } from '../libs/tag';
import { toError } from '../libs/utils';
import {
  QueryTestVariantsRequest,
  QueryTestVariantsResponse,
  ResultDb,
  TestVariant,
  TestVariantStatus,
} from '../services/resultdb';

/**
 * The stage of the next test variant. The stage can be
 * 1. LoadingXXX: the status of the next test variant will be no worse than XXX.
 * 2. Done: all test variants have been loaded.
 */
export const enum LoadingStage {
  LoadingUnexpected = 0,
  LoadingUnexpectedlySkipped = 1,
  LoadingFlaky = 2,
  LoadingExonerated = 3,
  LoadingExpected = 4,
  Done = 5,
}

export class LoadTestVariantsError extends Error implements InnerTag {
  readonly [TAG_SOURCE]: Error;

  constructor(readonly req: QueryTestVariantsRequest, source: Error) {
    super(source.message);
    makeObservable(this);

    this[TAG_SOURCE] = source;
  }
}

/**
 * Keeps the progress of the iterator and loads tests into the test node on
 * request.
 */
export class TestLoader {
  @observable.ref filter = (_v: TestVariant) => true;
  @observable.ref groupers: Array<[string, (v: TestVariant) => unknown]> = [];
  @observable.ref cmpFn = (_v1: TestVariant, _v2: TestVariant) => 0;

  @computed get isLoading() {
    return !this.loadedAllVariants && this.loadingReqCount !== 0;
  }
  @observable.ref private loadingReqCount = 0;

  /**
   * The queryTestVariants RPC sorted the variant by status. We can use this to
   * tell the possible status of the next test variants and therefore avoid
   * unnecessary loading.
   */
  @computed get stage() {
    return this._stage;
  }
  @observable.ref private _stage = LoadingStage.LoadingUnexpected;

  @observable.ref unfilteredTestVariantCount = 0;

  @computed
  get testVariantCount() {
    return this.nonExpectedTestVariants.length + this.expectedTestVariants.length;
  }

  /**
   * non-expected test variants grouped by keys from groupByPropGetters.
   * expected test variants are not included.
   */
  @computed get groupedNonExpectedVariants() {
    if (this.nonExpectedTestVariants.length === 0) {
      return [];
    }

    let groups = [this.nonExpectedTestVariants];
    for (const [, propGetter] of this.groupers) {
      groups = groups.flatMap((group) => Object.values(groupBy(group, propGetter)));
    }
    return groups.map((group) => group.sort(this.cmpFn));
  }

  @computed get groupedUnfilteredUnexpectedVariants() {
    if (this.unfilteredUnexpectedVariants.length === 0) {
      return [];
    }

    let groups = [this.unfilteredUnexpectedVariants];
    for (const [prop, propGetter] of this.groupers) {
      // No point grouping by status.
      if (prop === 'status') {
        continue;
      }
      groups = groups.flatMap((group) => Object.values(groupBy(group, propGetter)));
    }
    return groups.map((group) => group.slice().sort(this.cmpFn));
  }

  /**
   * non-expected test variants include test variants of any status except
   * TestVariantStatus.Expected.
   */
  @computed get nonExpectedTestVariants() {
    return this.unfilteredNonExpectedVariants.filter(this.filter);
  }
  @computed get unexpectedTestVariants() {
    return this.unfilteredUnexpectedVariants.filter(this.filter);
  }
  @computed get expectedTestVariants() {
    return this.unfilteredExpectedVariants.filter(this.filter);
  }

  @computed get unfilteredUnexpectedVariantsCount() {
    return this.unfilteredUnexpectedVariants.length;
  }

  @observable.ref private _unfilteredUnexpectedlySkippedVariantsCount = 0;
  get unfilteredUnexpectedlySkippedVariantsCount() {
    return this._unfilteredUnexpectedlySkippedVariantsCount;
  }
  @observable.ref private _unfilteredFlakyVariantsCount = 0;
  get unfilteredFlakyVariantsCount() {
    return this._unfilteredFlakyVariantsCount;
  }

  @observable.shallow private unfilteredNonExpectedVariants: TestVariant[] = [];
  @observable.shallow private unfilteredUnexpectedVariants: TestVariant[] = [];
  @observable.shallow private unfilteredExpectedVariants: TestVariant[] = [];

  @computed get loadedAllVariants() {
    return this.stage === LoadingStage.Done;
  }
  @computed get loadedAllUnexpectedVariants() {
    return this.stage > LoadingStage.LoadingUnexpected;
  }
  @computed get firstPageLoaded() {
    return this.unfilteredUnexpectedVariants.length > 0 || this.loadedAllUnexpectedVariants;
  }
  @computed get firstPageIsEmpty() {
    return this.loadedAllUnexpectedVariants && this.unfilteredUnexpectedVariants.length === 0;
  }

  // undefined means the end has been reached.
  // empty string is the token for the first page.
  private nextPageToken: string | undefined = '';

  constructor(private readonly req: QueryTestVariantsRequest, private readonly resultDb: ResultDb) {
    makeObservable(this);
  }

  private loadPromise = Promise.resolve();
  private firstLoadPromise?: Promise<void>;

  loadFirstPageOfTestVariants() {
    return this.firstLoadPromise || this.loadNextTestVariants();
  }

  /**
   * Load at least one test variant unless the last page is reached.
   */
  loadNextTestVariants() {
    if (this.stage === LoadingStage.Done) {
      return this.loadPromise;
    }

    this.loadingReqCount++;
    this.loadPromise = this.loadPromise
      .then(() => this.loadNextTestVariantsInternal())
      .then(() => {
        this.loadingReqCount--;
      });
    if (!this.firstLoadPromise) {
      this.firstLoadPromise = this.loadPromise;
    }

    return this.loadPromise;
  }

  /**
   * Load at least one test variant unless the last page is reached.
   *
   * @precondition there should not exist a running instance of
   * this.loadNextTestVariantsInternal
   */
  private async loadNextTestVariantsInternal() {
    const beforeCount = this.testVariantCount;

    // Load pages until the next expected status is at least the one we're after.
    do {
      await this.loadNextPage();
    } while (!this.loadedAllVariants && this.testVariantCount === beforeCount);
  }

  /**
   * Loads the next batch of tests from the iterator to the node.
   *
   * @precondition there should not exist a running instance of
   * this.loadMoreInternal
   */
  private async loadNextPage() {
    if (this.nextPageToken === undefined) {
      return;
    }

    const req = { ...this.req, pageToken: this.nextPageToken };
    let res: QueryTestVariantsResponse;
    try {
      res = await this.resultDb.queryTestVariants(req);
    } catch (e) {
      throw new LoadTestVariantsError(req, toError(e));
    }

    this.nextPageToken = res.nextPageToken;

    const testVariants = res.testVariants || [];
    this.processTestVariants(testVariants);
    if (this.nextPageToken === undefined) {
      this._stage = LoadingStage.Done;
      return;
    }
    if (testVariants.length < (this.req.pageSize || 1000)) {
      // When the service returns an incomplete page and nextPageToken is not
      // undefined, the following pages must be expected test variants.
      // Without this special case, the UI may incorrectly indicate that not all
      // variants have been loaded for statuses worse than Expected.
      this._stage = LoadingStage.LoadingExpected;
      return;
    }
  }

  private readonly stepsWithUnexpectedResults = new Set<string>();

  /**
   * Returns true if the step has unexpected, non-exonerated test results.
   * Returns false otherwise.
   *
   * This function may return false negatives when tests are not all loaded. But
   * because builds typically take longer to load than tests, it's unlikely for
   * the users to click on a step entry before all unexpected test results are
   * loaded.
   *
   * TODO(weiweilin): delete this once we no longer needs it to record GA
   * events.
   */
  hasAssociatedUnexpectedResults(stepName: string) {
    return this.stepsWithUnexpectedResults.has(stepName);
  }

  /**
   * Adds the associated steps of the test variant to stepsWithFailedResults.
   */
  private recordStep(v: TestVariant) {
    v.results?.forEach((r) => {
      const stepName = r.result.tags?.find((t) => t.key === 'step_name')?.value;
      if (stepName !== undefined) {
        this.stepsWithUnexpectedResults.add(stepName);
      }
    });
  }

  @action
  private processTestVariants(testVariants: readonly TestVariant[]) {
    this.unfilteredTestVariantCount += testVariants.length;
    for (const testVariant of testVariants) {
      switch (testVariant.status) {
        case TestVariantStatus.UNEXPECTED:
          this._stage = LoadingStage.LoadingUnexpected;
          this.unfilteredUnexpectedVariants.push(testVariant);
          this.unfilteredNonExpectedVariants.push(testVariant);
          this.recordStep(testVariant);
          break;
        case TestVariantStatus.UNEXPECTEDLY_SKIPPED:
          this._stage = LoadingStage.LoadingUnexpectedlySkipped;
          this._unfilteredUnexpectedlySkippedVariantsCount++;
          this.unfilteredNonExpectedVariants.push(testVariant);
          this.recordStep(testVariant);
          break;
        case TestVariantStatus.FLAKY:
          this._stage = LoadingStage.LoadingFlaky;
          this._unfilteredFlakyVariantsCount++;
          this.unfilteredNonExpectedVariants.push(testVariant);
          this.recordStep(testVariant);
          break;
        case TestVariantStatus.EXONERATED:
          this._stage = LoadingStage.LoadingExonerated;
          this.unfilteredNonExpectedVariants.push(testVariant);
          break;
        case TestVariantStatus.EXPECTED:
          this._stage = LoadingStage.LoadingExpected;
          this.unfilteredExpectedVariants.push(testVariant);
          break;
        default:
          break;
      }
    }
  }
}
