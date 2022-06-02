// Copyright 2022 The LUCI Authors.
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

import { TestHistoryService, Variant, VariantPredicate } from '../services/weetbix';

export class VariantLoader {
  /**
   * VariantHash -> Variant
   */
  @observable.shallow private readonly cache: Array<readonly [string, Variant]> = [];

  /**
   * The worker that populates `verdictsCache`.
   */
  private readonly worker: AsyncIterableIterator<null>;

  @computed get variants(): ReadonlyArray<readonly [string, Variant]> {
    return this.cache;
  }

  constructor(
    readonly project: string,
    readonly subRealm: string,
    readonly testId: string,
    readonly variantPredicate: VariantPredicate,
    readonly testHistoryService: TestHistoryService
  ) {
    this.worker = this.workerGen();
  }

  /**
   * Return a generator loads new variants.
   */
  private async *workerGen() {
    let pageToken = '';
    for (;;) {
      const res = await this.testHistoryService.queryVariants({
        project: this.project,
        subRealm: this.subRealm,
        testId: this.testId,
        variantPredicate: this.variantPredicate,
        pageToken,
      });

      const variantInfos = res.variants || [];
      for (const info of variantInfos) {
        this.cache.push([info.variantHash, info.variant]);
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

  getVariant(variantHash: string): Variant | null {
    return this.variants.find(([vHash]) => vHash === variantHash)?.[1] || null;
  }
}
