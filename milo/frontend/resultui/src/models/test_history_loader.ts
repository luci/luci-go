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
import { computed, observable } from 'mobx';

import { ResultDb, Variant } from '../services/resultdb';
import { TestHistoryService, TestVerdict } from '../services/weetbix';
import { TestHistoryVariantLoader } from './test_history_variant_loader';

export class TestHistoryLoader {
  /**
   * variant hash -> TestHistoryVariantLoader
   */
  @observable private readonly loaders = new Map<string, TestHistoryVariantLoader>();

  /**
   * The worker that drives variant discovery.
   */
  private readonly worker: AsyncIterableIterator<null>;

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
    readonly testHistoryService: TestHistoryService,
    readonly resultDb: ResultDb
  ) {
    this.worker = this.workerGen();
  }

  /**
   * Return a generator that drives the discovery of new variants.
   */
  private async *workerGen() {
    let pageToken = '';
    const [project, subRealm] = this.realm.split(':', 2);
    for (;;) {
      const res = await this.testHistoryService.queryVariants({
        project,
        subRealm,
        testId: this.testId,
        pageToken,
      });

      if (res.variants?.length) {
        for (const utv of res.variants) {
          let loader = this.loaders.get(utv.variantHash);
          if (!loader) {
            // If the variant is new, add a new loader for that variant.
            loader = new TestHistoryVariantLoader(
              this.realm,
              this.testId,
              utv.variant || { def: {} },
              this.resolve,
              this.testHistoryService
            );
            this.loaders.set(utv.variantHash, loader);
          }
        }
      }

      if (!res.nextPageToken) {
        return;
      }

      pageToken = res.nextPageToken;

      yield null;
    }
  }

  /**
   * Load all unique test variants associated with the test.
   *
   * Return true if all variants have been discovered.
   * Return false otherwise.
   */
  async discoverVariants() {
    const next = await this.worker.next();
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
  getEntries(variantHash: string, time: DateTime, noLoading = false): readonly TestVerdict[] | null {
    const vLoader = computed(() => this.loaders.get(variantHash)).get();
    if (!vLoader) {
      return null;
    }

    return vLoader.getEntries(time, noLoading);
  }
}
