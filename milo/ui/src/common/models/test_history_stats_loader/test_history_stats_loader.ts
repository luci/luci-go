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

import { DateTime } from 'luxon';
import { makeObservable, observable } from 'mobx';

import {
  QueryTestHistoryStatsRequest,
  QueryTestHistoryStatsResponseGroup,
  TestHistoryService,
  VariantPredicate,
} from '@/common/services/luci_analysis';

/**
 * A helper class that facilitate loading the test history stats and retrieving
 * test history stats associated with a (variant hash, date index) pair.
 *
 * The date index is defined as the number of days between the latestDate and
 * the given timestamp. e.g. If the latest date is 2022-01-05, the date index
 * of 2022-01-03 is 2.
 */
export class TestHistoryStatsLoader {
  readonly latestDate: DateTime;

  /**
   * Stats for test verdicts with a date index < `unloadedDateIndex` are all
   * loaded.
   * Stats for test verdicts with a date index > `unloadedDateIndex` are yet to
   * be loaded.
   * Stats for test verdicts with a date index === `unloadedDateIndex` might be
   * partially loaded.
   */
  @observable.ref private unloadedDateIndex = 0;

  /**
   * When `this.worker.next()` is called, it will keep loading until
   * `unloadedDateIndex` > `targetDateIndex`.
   */
  @observable.ref private targetDateIndex = 0;

  /**
   * `${variantHash}/${dateIndex}` -> stats
   */
  @observable.shallow private readonly cache = new Map<
    string,
    QueryTestHistoryStatsResponseGroup
  >();

  /**
   * The worker that populates `cache`.
   */
  private readonly worker: AsyncIterableIterator<null>;

  constructor(
    readonly project: string,
    readonly subRealm: string,
    readonly testId: string,
    latestDate: DateTime,
    readonly variantPredicate: VariantPredicate,
    readonly testHistoryService: TestHistoryService,
  ) {
    makeObservable(this);

    this.latestDate = latestDate.toUTC().startOf('day');
    this.worker = this.workerGen();
  }

  private async *workerGen() {
    const req: QueryTestHistoryStatsRequest = {
      project: this.project,
      predicate: {
        subRealm: this.subRealm,
        partitionTimeRange: {
          // -1 day so all the results partitioned within 24hrs since the
          // `latestDate` will be included.
          latest: this.latestDate.minus({ day: -1 }).toISO(),
        },
        variantPredicate: this.variantPredicate,
      },
      pageSize: 1000,
      testId: this.testId,
    };
    let pageToken = '';

    for (;;) {
      // We've loaded all required entries. Yield back.
      while (this.unloadedDateIndex > this.targetDateIndex) {
        yield null;
      }
      const res = await this.testHistoryService.queryStats({
        ...req,
        pageToken,
      });

      const groups = res.groups || [];
      for (const group of groups) {
        const dateIndex = this.getDateIndex(
          DateTime.fromISO(group.partitionTime),
        );
        this.cache.set(`${group.variantHash}/${dateIndex}`, group);
      }

      if (!res.nextPageToken) {
        this.unloadedDateIndex = Infinity;
        return;
      }

      pageToken = res.nextPageToken;

      if (groups.length > 0) {
        this.unloadedDateIndex = this.getDateIndex(
          DateTime.fromISO(groups[groups.length - 1].partitionTime),
        );
      }
    }
  }

  /**
   * Get all entries associated with the specified variant hash and date index.
   *
   * If the entries associated with the specified variant hash and date index
   * hasn't been loaded yet, return null.
   */
  getStats(
    variantHash: string,
    dateIndex: number,
    noLoading = false,
  ): QueryTestHistoryStatsResponseGroup | null {
    // If the dateIndex is not loaded, return null.
    // For simplicity, treat partially loaded date as not loaded. Even though
    // entries for some variants might have been loaded already.
    if (dateIndex >= this.unloadedDateIndex) {
      if (!noLoading) {
        this.loadUntil(dateIndex);
      }
      return null;
    }

    const stats = this.cache.get(`${variantHash}/${dateIndex}`);
    if (!stats) {
      // This variant hash doesn't exist. Return empty stats.
      return {
        variantHash,
        partitionTime: this.getDateTime(dateIndex).toISO(),
        verdictCounts: {},
      };
    }

    return stats;
  }

  /**
   * Load all entries that were created after `dateIndex`.
   *
   * See the documentation of `getStats` for the definition of dateIndex.
   */
  async loadUntil(dateIndex: number) {
    if (dateIndex > this.targetDateIndex) {
      this.targetDateIndex = dateIndex;
    }
    await this.worker.next();
    return;
  }

  /**
   * Get the date index of a given time.
   */
  private getDateIndex(time: DateTime): number {
    return this.latestDate.diff(time).as('days');
  }

  /**
   * Get the DateTime object of a given date index.
   *
   * It's an inverse function of `getDateIndex`.
   */
  private getDateTime(dateIndex: number): DateTime {
    return this.latestDate.minus({ day: dateIndex });
  }
}
