// Copyright 2021 The LUCI Authors.
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

import { computed, observable } from 'mobx';

import { TestVariant } from '../services/resultdb';
import { TestHistoryService, TestVariantHistoryEntry } from '../services/test_history_service';

/**
 * A utility class that helps loading test history entry details.
 */
export class TestHistoryEntriesLoader {
  constructor(
    readonly testId: string,
    readonly tvhEntries: readonly TestVariantHistoryEntry[],
    readonly testHistoryService: TestHistoryService,
    readonly pageSize = 10,
  ) {}

  @observable.shallow private _testVariants: TestVariant[] = [];
  @computed get testVariants(): readonly TestVariant[] {
    return this._testVariants;
  }

  private loadPromise = Promise.resolve();
  private firstLoadPromise?: Promise<void>;

  @observable.ref private loadingReqCount = 0;
  get isLoading() {
    return this.loadingReqCount !== 0;
  }
  @computed get loadedAllTestVariants() {
    return this.tvhEntries.length === this._testVariants.length;
  }
  @computed get loadedFirstPage() {
    return this._testVariants.length > 0;
  }

  /**
   * Loads the next batch of tests.
   *
   * @precondition there should not exist a running instance of
   * this.loadNextPage
   */
  private async loadNextPageImpl() {
    const loadCount = Math.min(this.tvhEntries.length - this._testVariants.length, this.pageSize);

    // Load all new entries in parallel.
    const newVariants = await Promise.all(
      Array(loadCount)
        .fill(0)
        .map((_, i) => {
          const entry = this.tvhEntries[this._testVariants.length + i];
          return this.testHistoryService.getTestVariant({
            testId: this.testId,
            invocationIds: entry.invocationIds,
            variant: entry.variant || { def: {} },
            variantHash: entry.variantHash,
          });
        })
    );

    this._testVariants.push(...newVariants);
  }

  // Don't mark as async so loadingReqCount and firstLoadPromise can be updated
  // immediately.
  loadNextPage(): Promise<void> {
    if (this.loadedAllTestVariants) {
      return this.loadPromise;
    }

    this.loadingReqCount++;
    this.loadPromise = this.loadPromise
      .then(() => this.loadNextPageImpl())
      .then(() => {
        this.loadingReqCount--;
      });
    if (!this.firstLoadPromise) {
      this.firstLoadPromise = this.loadPromise;
    }

    return this.loadPromise;
  }

  loadFirstPage(): Promise<void> {
    return this.firstLoadPromise || this.loadNextPage();
  }
}
