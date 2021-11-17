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

import { DateTime } from 'luxon';
import { comparer, computed, observable, untracked } from 'mobx';

import { Variant } from '../services/resultdb';
import { TestHistoryService, TestVariantHistoryEntry } from '../services/test_history_service';

export class TestHistoryLoader {
  @observable private _variants = new Map<string, Variant>();
  @computed.struct get variants(): readonly [string, Variant][] {
    return [...this._variants.entries()];
  }

  /**
   * variant hash -> (datetime str -> test variant history entries)
   */
  private readonly cache = new Map<string, Map<string, TestVariantHistoryEntry[]>>();

  /**
   * Test histories created after `loadedTime` are all loaded.
   * Test histories created before `loadedTime` are yet to be loaded.
   */
  // Initialize to a future date. All the entries created after 1 year in the
  // future can be considered loaded because we know they don't exist.
  @observable.ref private loadedTime = DateTime.now().plus({ years: 1 });
  @computed private get loadedTimeStr() {
    return this.resolve(this.loadedTime);
  }

  /**
   * When `this.worker.next()` is called, it will keep loading until
   * 1. `loadedTime` < `targetTime`, and
   * 2. `loadedTime` and `targetTime` resolves to different strings.
   */
  private targetTime = this.loadedTime;
  @computed private get targetTimeStr() {
    return this.resolve(this.targetTime);
  }

  private readonly worker: AsyncIterableIterator<null>;

  constructor(
    readonly realm: string,
    readonly testId: string,
    /**
     * Resolve controls the size of the time step when grouping and querying
     * test history entries. For example, if all timestamps between
     * [2021-11-05T00:00:00Z, 2021-11-06T00:00:00Z) resolves to '2021-11-05',
     * All test history entries in that time range will be grouped together.
     * They will all be returned when `getEntries` is called with a timestamp
     * between [2021-11-05T00:00:00Z, 2021-11-06T00:00:00Z).
     *
     * Note: if time1 and time2 both resolve to the same string, any time
     * between time1 and time2 must resolves to the same string.
     */
    readonly resolve: (time: DateTime) => string,
    readonly testHistoryService: TestHistoryService
  ) {
    this.worker = this.workerGen();
  }

  private async *workerGen() {
    let pageToken = '';
    for (;;) {
      const res = await this.testHistoryService.queryTestHistory({
        realm: this.realm,
        testId: this.testId,
        timeRange: {},
        pageToken: pageToken,
      });

      for (const entry of res.entries) {
        let variantCache = this.cache.get(entry.variantHash);
        if (!variantCache) {
          this._variants.set(entry.variantHash, entry.variant || { def: {} });
          variantCache = new Map<string, TestVariantHistoryEntry[]>();
          this.cache.set(entry.variantHash, variantCache);
        }

        this.loadedTime = DateTime.fromISO(entry.invocationTimestamp);
        let dateCache = variantCache.get(this.loadedTimeStr);
        if (!dateCache) {
          dateCache = [];
          variantCache.set(this.loadedTimeStr, dateCache);
        }

        dateCache.push(entry);
      }

      if (!res.nextPageToken) {
        // We've loaded all the entries. Set the loaded time to the earliest
        // possible time.
        this.loadedTime = DateTime.fromMillis(0);
        return;
      }

      pageToken = res.nextPageToken;

      // We've loaded all required entries. Yield back.
      while (this.targetTime > this.loadedTime && this.targetTimeStr !== this.loadedTimeStr) {
        yield null;
      }
    }
  }

  /**
   * Load all entries that were created after `time`.
   */
  async loadUntil(time: DateTime) {
    if (time >= this.targetTime) {
      return;
    }
    this.targetTime = time;
    await this.worker.next();
    return;
  }

  /**
   * Get all entries associated with the specified variant hash and time slot.
   *
   * If there are no entries associated with specified variant hash and time
   * slot, return an empty array.
   * If the entries associated with the specified variant hash and time slot
   * hasn't been loaded yet, return null.
   */
  getEntries(variantHash: string, time: DateTime, noLoading = false): TestVariantHistoryEntry[] | null {
    const timeStr = this.resolve(time);
    const loaded = computed(() => time > this.loadedTime && this.loadedTimeStr !== timeStr).get();
    if (!loaded) {
      if (!noLoading) {
        untracked(() => this.loadUntil(time));
      }
      return null;
    }
    const dateStr = this.resolve(time);
    const ret = computed(() => this.cache.get(variantHash)?.get(dateStr) || [], { equals: comparer.shallow }).get();
    return ret;
  }
}
