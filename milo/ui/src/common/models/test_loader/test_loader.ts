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

import {
  QueryTestVariantsRequest,
  QueryTestVariantsResponse,
  ResultDb,
  TestVariant,
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from '@/common/services/resultdb';
import { InnerTag, TAG_SOURCE } from '@/generic_libs/tools/tag';
import { toError } from '@/generic_libs/tools/utils';

/**
 * The stage of the next test variant. The stage can be
 * 1. LoadingXXX: the status of the next test variant will be no worse than XXX.
 * 2. Done: all test variants have been loaded.
 */
export const enum LoadingStage {
  LoadingFailedVerdicts = 0,
  LoadingExecutionErroredVerdicts = 1,
  LoadingPrecludedVerdicts = 2,
  LoadingFlakyVerdicts = 3,
  LoadingExoneratedVerdicts = 4,
  LoadingPassedAndSkippedVerdicts = 5,
  Done = 6,
}

export class LoadTestVariantsError extends Error implements InnerTag {
  readonly [TAG_SOURCE]: Error;

  constructor(
    readonly req: QueryTestVariantsRequest,
    source: Error,
  ) {
    super(source.message);

    this[TAG_SOURCE] = source;
  }
}

/**
 * Keeps the progress of the iterator and loads tests into the test node on
 * request.
 */
export class TestLoader {
  @observable.ref filter = (_v: TestVariant) => true;
  @observable.ref groupers: ReadonlyArray<
    readonly [string, (v: TestVariant) => unknown]
  > = [];
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
  @observable.ref private _stage = LoadingStage.LoadingFailedVerdicts;

  @observable.ref unfilteredTestVariantCount = 0;

  @computed
  get testVariantCount() {
    return (
      this.nonPassedNorSkippedTestVariants.length +
      this.passedAndSkippedTestVariants.length
    );
  }

  /**
   * not passed or skipped test variants grouped by keys from groupByPropGetters.
   * passed and skipped test variants are not included.
   */
  @computed get groupedNonPassedNorSkippedVariants() {
    if (this.nonPassedNorSkippedTestVariants.length === 0) {
      return [];
    }

    let groups = [this.nonPassedNorSkippedTestVariants];
    for (const [, propGetter] of this.groupers) {
      groups = groups.flatMap((group) =>
        Object.values(groupBy(group, propGetter)),
      );
    }
    return groups.map((group) => group.sort(this.cmpFn));
  }

  @computed get groupedUnfilteredFailedVariants() {
    if (this.unfilteredFailedVariants.length === 0) {
      return [];
    }

    let groups = [this.unfilteredFailedVariants];
    for (const [prop, propGetter] of this.groupers) {
      // No point grouping by status.
      if (prop === 'status') {
        continue;
      }
      groups = groups.flatMap((group) =>
        Object.values(groupBy(group, propGetter)),
      );
    }
    return groups.map((group) => group.slice().sort(this.cmpFn));
  }

  /**
   * Include test variants of any status except Passed or Skipped.
   */
  @computed get nonPassedNorSkippedTestVariants() {
    return this.unfilteredNonPassedNorSkippedVariants.filter(this.filter);
  }
  @computed get failedTestVariants() {
    return this.unfilteredFailedVariants.filter(this.filter);
  }
  @computed get passedAndSkippedTestVariants() {
    return this.unfilteredPassedAndSkippedVariants.filter(this.filter);
  }

  @computed get unfilteredFailedVariantsCount() {
    return this.unfilteredFailedVariants.length;
  }

  @observable.ref private _unfilteredExecutionErroredVariantsCount = 0;
  get unfilteredExecutionErroredVariantsCount() {
    return this._unfilteredExecutionErroredVariantsCount;
  }
  @observable.ref private _unfilteredPrecludedVariantsCount = 0;
  get unfilteredPrecludedVariantsCount() {
    return this._unfilteredPrecludedVariantsCount;
  }
  @observable.ref private _unfilteredFlakyVariantsCount = 0;
  get unfilteredFlakyVariantsCount() {
    return this._unfilteredFlakyVariantsCount;
  }

  @observable.shallow
  private unfilteredNonPassedNorSkippedVariants: TestVariant[] = [];
  @observable.shallow private unfilteredFailedVariants: TestVariant[] = [];
  @observable.shallow
  private unfilteredPassedAndSkippedVariants: TestVariant[] = [];

  @computed get loadedAllVariants() {
    return this.stage === LoadingStage.Done;
  }
  @computed get loadedAllFailedVariants() {
    return this.stage > LoadingStage.LoadingFailedVerdicts;
  }
  @computed get firstPageLoaded() {
    return (
      this.unfilteredFailedVariants.length > 0 || this.loadedAllFailedVariants
    );
  }
  @computed get firstPageIsEmpty() {
    return (
      this.loadedAllFailedVariants && this.unfilteredFailedVariants.length === 0
    );
  }

  // undefined means the end has been reached.
  // empty string is the token for the first page.
  private nextPageToken: string | undefined = '';

  constructor(
    private readonly req: QueryTestVariantsRequest,
    private readonly resultDb: ResultDb,
  ) {
    makeObservable(this);
  }

  private loadPromise = Promise.resolve();
  private firstLoadPromise?: Promise<void>;

  loadFirstPageOfTestVariants() {
    // Use a smaller page size when loading the first page for faster page load.
    // After that, use a larger page size because that reduce the number of
    // roundtrips when doing client-side search.
    return this.firstLoadPromise || this.loadNextTestVariants(1000);
  }

  /**
   * Load at least one test variant unless the last page is reached.
   */
  @action
  loadNextTestVariants(pageSize = 10000) {
    if (this.stage === LoadingStage.Done) {
      return this.loadPromise;
    }

    this.loadingReqCount++;
    this.loadPromise = this.loadPromise
      .then(() => this.loadNextTestVariantsInternal(pageSize))
      .then(
        action(() => {
          this.loadingReqCount--;
        }),
      );
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
  private async loadNextTestVariantsInternal(pageSize: number) {
    const beforeCount = this.testVariantCount;

    // Load pages until the next expected status is at least the one we're after.
    do {
      await this.loadNextPage(pageSize);
    } while (!this.loadedAllVariants && this.testVariantCount === beforeCount);
  }

  /**
   * Loads the next batch of tests from the iterator to the node.
   *
   * @precondition there should not exist a running instance of
   * this.loadMoreInternal
   */
  private async loadNextPage(pageSize: number) {
    if (this.nextPageToken === undefined) {
      return;
    }

    const req = {
      ...this.req,
      pageSize,
      pageToken: this.nextPageToken,
      orderBy: 'status_v2_effective',
    };
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
      action(() => (this._stage = LoadingStage.Done))();
      return;
    }
    if (testVariants.length < (this.req.pageSize || 1000)) {
      // When the service returns an incomplete page and nextPageToken is not
      // undefined, the following pages must be expected test variants.
      // Without this special case, the UI may incorrectly indicate that not all
      // variants have been loaded for statuses worse than Expected.
      action(
        () => (this._stage = LoadingStage.LoadingPassedAndSkippedVerdicts),
      )();
      return;
    }
  }

  private readonly stepsWithUnexpectedResults = new Set<string>();

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
      const effectiveStatus =
        testVariant.statusOverride === TestVerdict_StatusOverride.EXONERATED
          ? testVariant.statusOverride
          : testVariant.statusV2;
      switch (effectiveStatus) {
        case TestVerdict_Status.FAILED:
          this._stage = LoadingStage.LoadingFailedVerdicts;
          this.unfilteredFailedVariants.push(testVariant);
          this.unfilteredNonPassedNorSkippedVariants.push(testVariant);
          this.recordStep(testVariant);
          break;
        case TestVerdict_Status.EXECUTION_ERRORED:
          this._stage = LoadingStage.LoadingExecutionErroredVerdicts;
          this._unfilteredExecutionErroredVariantsCount++;
          this.unfilteredNonPassedNorSkippedVariants.push(testVariant);
          this.recordStep(testVariant);
          break;
        case TestVerdict_Status.PRECLUDED:
          this._stage = LoadingStage.LoadingPrecludedVerdicts;
          this._unfilteredPrecludedVariantsCount++;
          this.unfilteredNonPassedNorSkippedVariants.push(testVariant);
          this.recordStep(testVariant);
          break;
        case TestVerdict_Status.FLAKY:
          this._stage = LoadingStage.LoadingFlakyVerdicts;
          this._unfilteredFlakyVariantsCount++;
          this.unfilteredNonPassedNorSkippedVariants.push(testVariant);
          this.recordStep(testVariant);
          break;
        case TestVerdict_StatusOverride.EXONERATED:
          this._stage = LoadingStage.LoadingExoneratedVerdicts;
          this.unfilteredNonPassedNorSkippedVariants.push(testVariant);
          break;
        case TestVerdict_Status.PASSED: // Fallthrough.
        case TestVerdict_Status.SKIPPED:
          this._stage = LoadingStage.LoadingPassedAndSkippedVerdicts;
          this.unfilteredPassedAndSkippedVariants.push(testVariant);
          break;
        default:
          break;
      }
    }
  }
}
