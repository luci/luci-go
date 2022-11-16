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
import { comparer, computed, makeObservable, observable, untracked } from 'mobx';

import { TestHistoryService, TestVerdict } from '../services/luci_analysis';
import { Variant } from '../services/resultdb';

/**
 * Test history loader for a specific variant.
 */
export class TestHistoryVariantLoader {
  /**
   * datetime str -> test variant history entries
   */
  private readonly cache = new Map<string, TestVerdict[]>();

  private worker: AsyncIterableIterator<null>;

  /**
   * Test histories created after `loadedTime` are all loaded.
   * Test histories created before `loadedTime` are yet to be loaded.
   */
  // Initialize to a future date. All the entries created after 1 year in the
  // future can be considered loaded because we know they don't exist.
  @observable.ref private loadedTime = DateTime.now().plus({ years: 1 });
  @computed private get loadedTimeGroupId() {
    return this.resolve(this.loadedTime);
  }

  /**
   * When `this.worker.next()` is called, it will keep loading until
   * 1. `loadedTime` < `targetTime`, and
   * 2. `loadedTime` and `targetTime` resolves to different strings.
   */
  @observable.ref private targetTime = this.loadedTime;
  @computed private get targetTimeGroupId() {
    return this.resolve(this.targetTime);
  }

  constructor(
    readonly realm: string,
    readonly testId: string,
    readonly variant: Variant,
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
    makeObservable(this);

    this.worker = this.workerGen();
  }

  /**
   * Generates a worker that loads the entries between
   * [`now`, `this.targetTime`] then yields back.
   *
   * `this.targetTime` can be updated so the worker can load the entries between
   * [`last target time`, `this.targetTime`] when `.next()` is called.
   */
  private async *workerGen() {
    let pageToken = '';
    for (;;) {
      // We've loaded all required entries. Yield back.
      while (this.targetTime > this.loadedTime && this.targetTimeGroupId !== this.loadedTimeGroupId) {
        yield null;
      }

      const [project, subRealm] = this.realm.split(':', 2);

      const res = await this.testHistoryService.query({
        project,
        testId: this.testId,
        predicate: {
          subRealm,
          variantPredicate: { equals: this.variant },
          partitionTimeRange: {},
        },
        pageToken: pageToken,
        pageSize: 1000,
      });

      for (const entry of res.verdicts || []) {
        this.addEntry(entry);
      }

      if (!res.nextPageToken) {
        // We've loaded all the entries. Set the loaded time to the earliest
        // possible time.
        this.loadedTime = DateTime.fromMillis(0);
        return;
      }

      pageToken = res.nextPageToken;
    }
  }

  /**
   * Adds the entry to the cache.
   */
  private addEntry(entry: TestVerdict) {
    this.loadedTime = DateTime.fromISO(entry.partitionTime);

    let dateCache = this.cache.get(this.loadedTimeGroupId);
    if (!dateCache) {
      dateCache = [];
      this.cache.set(this.loadedTimeGroupId, dateCache);
    }

    dateCache.push(entry);
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
   * Get all entries associated with the time slot.
   *
   * If there are no entries associated with time slot, return an empty array.
   * If the entries associated with the time slot hasn't been loaded yet, return
   * null.
   */
  getEntries(time: DateTime, noLoading = false): readonly TestVerdict[] | null {
    const timeStr = this.resolve(time);
    const loaded = computed(() => time > this.loadedTime && this.loadedTimeGroupId !== timeStr).get();
    if (!loaded) {
      if (!noLoading) {
        untracked(() => this.loadUntil(time));
      }
      return null;
    }
    const dateStr = this.resolve(time);
    const ret = computed(() => this.cache.get(dateStr) || [], { equals: comparer.shallow }).get();
    return ret;
  }
}
