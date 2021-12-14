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

import stableStringify from 'fast-json-stable-stringify';
import { groupBy } from 'lodash-es';
import { DateTime } from 'luxon';
import { computed, observable } from 'mobx';

import { Variant, VariantPredicate } from '../services/resultdb';
import { TestHistoryService, TestVariantHistoryEntry } from '../services/test_history_service';
import { TestHistoryVariantLoader } from './test_history_variant_loader';

export class TestHistoryLoader {
  /**
   * variant hash -> TestHistoryVariantLoader
   */
  @observable private readonly loaders = new Map<string, TestHistoryVariantLoader>();

  /**
   * variant predicate hash -> Worker.
   */
  private workers = new Map<string, AsyncIterableIterator<null>>();

  @computed.struct get variants(): readonly [string, Variant][] {
    return [...this.loaders.entries()].map(([hash, loader]) => [hash, loader.variant]);
  }

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
  ) {}

  /**
   * Return a generator that drives the discovery of new variants.
   */
  private async *workerGen(predicate: VariantPredicate | undefined) {
    const visitedLoaders = new Set<TestHistoryVariantLoader>();
    let pageToken = '';
    for (;;) {
      const res = await this.testHistoryService.queryTestHistory({
        realm: this.realm,
        testId: this.testId,
        timeRange: {},
        variantPredicate: predicate,
        pageToken: pageToken,
      });

      if (res.entries.length) {
        const loadedTime = DateTime.fromISO(res.entries[res.entries.length - 1].invocationTimestamp);
        const groupedEntries = groupBy(res.entries, (entry) => entry.variantHash);

        for (const [hash, group] of Object.entries(groupedEntries)) {
          let loader = this.loaders.get(hash);
          if (!loader) {
            // If the variant is new, add a new loader for that variant.
            loader = new TestHistoryVariantLoader(
              this.realm,
              this.testId,
              group[0].variant || { def: {} },
              this.resolve,
              this.testHistoryService
            );
            this.loaders.set(hash, loader);
          }

          // We mainly load those entries to get the variant definitions. But we
          // can also reuse those entries in variant loaders.
          loader.populateEntries(group, loadedTime);
          visitedLoaders.add(loader);
        }
      }

      if (!res.nextPageToken) {
        // Since we reached the end of the page, all the variants this loader
        // encountered can be considered finalized.
        const after = DateTime.fromMillis(0);
        for (const loader of visitedLoaders) {
          loader.populateEntries([], after);
        }

        return;
      }

      pageToken = res.nextPageToken;

      yield null;
    }
  }

  /**
   * Load variants that matches the predicate.
   *
   * Return true if all variants matches the predicate are discovered.
   * Return false otherwise.
   */
  async discoverVariants(predicate: VariantPredicate | undefined, pages = 1) {
    const hash = stableStringify(predicate);
    let worker = this.workers.get(hash);
    if (!worker) {
      worker = this.workerGen(predicate);
      this.workers.set(hash, worker);
    }

    let next = await worker.next();
    while (pages > 1 && !next.done) {
      next = await worker.next();
      pages--;
    }
    return Boolean(next.done);
  }

  /**
   * Get all entries associated with the specified variant hash and time slot.
   *
   * If there are no entries associated with specified variant hash and time
   * slot, return an empty array.
   * If the entries associated with the specified variant hash and time slot
   * hasn't been loaded yet, return null.
   */
  getEntries(variantHash: string, time: DateTime, noLoading = false): readonly TestVariantHistoryEntry[] | null {
    const vLoader = computed(() => this.loaders.get(variantHash)).get();
    if (!vLoader) {
      return null;
    }

    return vLoader.getEntries(time, noLoading);
  }
}
