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

import { interpolateOranges, scaleLinear, scaleSequential } from 'd3';
import { DateTime } from 'luxon';
import { autorun, comparer, computed } from 'mobx';
import { addDisposer, types } from 'mobx-state-tree';

import { PageLoader } from '@/common/models/page_loader';
import { TestHistoryStatsLoader } from '@/common/models/test_history_stats_loader';
import {
  parseVariantFilter,
  parseVariantPredicate,
  VariantFilter,
} from '@/common/queries/th_filter_query';
import {
  QueryTestHistoryStatsResponseGroup,
  TestVerdictBundle,
  TestVerdictStatus,
  Variant,
  VariantPredicate,
} from '@/common/services/luci_analysis';
import { ServicesStore } from '@/common/store/services';
import { Timestamp } from '@/common/store/timestamp';
import { logging } from '@/common/tools/logging';
import { keepAliveComputed } from '@/generic_libs/tools/mobx_utils';
import { getCriticalVariantKeys } from '@/test_verdict/tools/variant_utils/variant_utils';

export const enum GraphType {
  STATUS = 'STATUS',
  DURATION = 'DURATION',
}

export const enum XAxisType {
  DATE = 'DATE',
  COMMIT = 'COMMIT',
}

// Use `scaleColor` to discard colors avoid using white color when the input is
// close to 0.
const scaleColor = scaleLinear().range([0.1, 1]).domain([0, 1]);

export const TestHistoryPage = types
  .model('TestHistoryPage', {
    refreshTime: types.safeReference(Timestamp),
    services: types.safeReference(ServicesStore),

    project: types.maybe(types.string),
    subRealm: types.maybe(types.string),
    testId: types.maybe(types.string),

    days: 14,
    filterText: '',

    selectedGroup: types.frozen<QueryTestHistoryStatsResponseGroup | null>(
      null,
    ),

    graphType: types.frozen<GraphType>(GraphType.STATUS),
    xAxisType: types.frozen<XAxisType>(XAxisType.DATE),

    countUnexpected: true,
    countUnexpectedlySkipped: true,
    countFlaky: true,
    countExonerated: true,

    durationInitialized: false,
    minDurationMs: 0,
    maxDurationMs: 100,

    defaultSortingKeys: types.frozen<readonly string[]>([]),
    customColumnKeys: types.frozen<readonly string[]>([]),
    customColumnWidths: types.frozen<{ readonly [key: string]: number }>({}),
    customSortingKeys: types.frozen<readonly string[]>([]),
  })
  .volatile(() => ({
    variantFilter: ((_v: Variant, _hash: string) => true) as VariantFilter,
    variantPredicate: { contains: { def: {} } } as VariantPredicate,
  }))
  .views((self) => ({
    get latestDate() {
      return (self.refreshTime?.dateTime || DateTime.now())
        .toUTC()
        .startOf('day');
    },
    get scaleDurationColor() {
      return scaleSequential((x) => interpolateOranges(scaleColor(x))).domain([
        self.minDurationMs,
        self.maxDurationMs,
      ]);
    },
  }))
  .views((self) => {
    const variantsLoader = keepAliveComputed(self, () => {
      if (!self.project || !self.testId || !self.services?.testHistory) {
        return null;
      }

      // Establish dependencies so the loader will get re-computed correctly.
      const project = self.project;
      const subRealm = self.subRealm;
      const testId = self.testId;
      const variantPredicate = self.variantPredicate;
      const testHistoryService = self.services.testHistory;

      return new PageLoader(async (pageToken) => {
        const res = await testHistoryService.queryVariants({
          project,
          subRealm,
          testId,
          variantPredicate,
          pageToken,
        });
        return [
          res.variants?.map(
            (v) =>
              [v.variantHash, v.variant || { def: {} }] as [string, Variant],
          ) || [],
          res.nextPageToken,
        ];
      });
    });

    const statsLoader = keepAliveComputed(self, () => {
      if (!self.project || !self.testId || !self.services?.testHistory) {
        return null;
      }
      return new TestHistoryStatsLoader(
        self.project,
        self.subRealm || '',
        self.testId,
        self.latestDate,
        self.variantPredicate,
        self.services.testHistory,
      );
    });

    const entriesLoader = keepAliveComputed(self, () => {
      if (
        !self.project ||
        !self.testId ||
        !self.selectedGroup?.variantHash ||
        !self.selectedGroup?.partitionTime ||
        !self.services?.testHistory
      ) {
        return null;
      }
      const varLoader = variantsLoader.get();
      if (!varLoader) {
        return null;
      }

      // Establish dependencies so the loader will get re-computed correctly.
      const project = self.project;
      const subRealm = self.subRealm;
      const testId = self.testId;
      const variantHash = self.selectedGroup?.variantHash;
      const earliest = self.selectedGroup?.partitionTime;
      const testHistoryService = self.services.testHistory;
      const variant = varLoader.items.find(
        ([vHash]) => vHash === variantHash,
      )![1];
      const latest = DateTime.fromISO(earliest).minus({ days: -1 }).toISO();

      return new PageLoader(async (pageToken) => {
        const res = await testHistoryService.query({
          project,
          testId,
          predicate: {
            subRealm,
            variantPredicate: {
              equals: variant,
            },
            partitionTimeRange: {
              earliest,
              latest,
            },
          },
          pageSize: 100,
          pageToken,
        });
        return [
          res.verdicts?.map((verdict) => ({ verdict, variant: variant })) || [],
          res.nextPageToken,
        ];
      });
    });

    return {
      get variantsLoader() {
        return variantsLoader.get();
      },
      get statsLoader() {
        return statsLoader.get();
      },
      get entriesLoader() {
        return entriesLoader.get();
      },
    };
  })
  .views((self) => {
    const criticalVariantKeys = computed(
      () =>
        getCriticalVariantKeys(
          self.variantsLoader?.items.map(([_, v]) => v) || [],
        ),
      { equals: comparer.shallow },
    );

    return {
      get filteredVariants() {
        return (
          self.variantsLoader?.items.filter(([hash, v]) =>
            self.variantFilter(v, hash),
          ) || []
        );
      },
      get criticalVariantKeys(): readonly string[] {
        return criticalVariantKeys.get();
      },
      get defaultColumnKeys(): readonly string[] {
        return this.criticalVariantKeys.map((k) => 'v.' + k);
      },
      get columnKeys(): readonly string[] {
        return self.customColumnKeys || this.defaultColumnKeys;
      },
      get columnWidths(): readonly number[] {
        return this.columnKeys.map(
          (col) => self.customColumnWidths[col] ?? 100,
        );
      },
      get sortingKeys(): readonly string[] {
        return self.customSortingKeys || self.defaultSortingKeys;
      },
      get verdictBundles() {
        if (!self.entriesLoader?.items.length) {
          return [];
        }

        const cmpFn = createTVCmpFn(this.sortingKeys);
        return [...(self.entriesLoader?.items || [])].sort(cmpFn);
      },
      get selectedTestVerdictCount() {
        return (
          (self.selectedGroup?.unexpectedCount || 0) +
          (self.selectedGroup?.unexpectedlySkippedCount || 0) +
          (self.selectedGroup?.flakyCount || 0) +
          (self.selectedGroup?.exoneratedCount || 0) +
          (self.selectedGroup?.expectedCount || 0)
        );
      },
    };
  })
  .actions((self) => ({
    setDependencies(deps: Pick<typeof self, 'refreshTime' | 'services'>) {
      Object.assign(self, deps);
    },
    setParams(projectOrRealm: string, testId: string) {
      [self.project, self.subRealm] = projectOrRealm.split(':', 2);
      self.testId = testId;
    },
    setFilterText(filterText: string) {
      self.filterText = filterText;
    },
    setSelectedGroup(group: QueryTestHistoryStatsResponseGroup | null) {
      self.selectedGroup = group;
    },
    setDays(days: number) {
      self.days = days;
    },
    setGraphType(type: GraphType) {
      self.graphType = type;
    },
    setXAxisType(type: XAxisType) {
      self.xAxisType = type;
    },
    setCountUnexpected(count: boolean) {
      self.countUnexpected = count;
    },
    setCountUnexpectedlySkipped(count: boolean) {
      self.countUnexpectedlySkipped = count;
    },
    setCountFlaky(count: boolean) {
      self.countFlaky = count;
    },
    setCountExonerated(count: boolean) {
      self.countExonerated = count;
    },
    setDuration(durationMs: number) {
      if (self.durationInitialized) {
        self.minDurationMs = durationMs;
        self.maxDurationMs = durationMs;
        return;
      }
      self.minDurationMs = Math.min(self.minDurationMs, durationMs);
      self.maxDurationMs = Math.max(self.maxDurationMs, durationMs);
    },
    resetDurations() {
      self.durationInitialized = false;
      self.minDurationMs = 0;
      self.maxDurationMs = 100;
    },
    setColumnKeys(v: readonly string[]) {
      self.customColumnKeys = v;
    },
    setColumnWidths(v: { readonly [key: string]: number }) {
      self.customColumnWidths = v;
    },
    setSortingKeys(v: readonly string[]): void {
      self.customSortingKeys = v;
    },
    _updateFilters(filter: VariantFilter, predicate: VariantPredicate) {
      self.variantFilter = filter;
      self.variantPredicate = predicate;
    },
    afterCreate() {
      addDisposer(
        self,
        autorun(() => {
          try {
            const newVariantFilter = parseVariantFilter(self.filterText);
            const newVariantPredicate = parseVariantPredicate(self.filterText);

            // Only update the filters after the query is successfully parsed.
            this._updateFilters(newVariantFilter, newVariantPredicate);
          } catch (e) {
            // TODO(weiweilin): display the error to the user.
            logging.error(e);
          }
        }),
      );
    },
  }));

// Note: once we have more than 9 statuses, we need to add '0' prefix so '10'
// won't appear before '2' after sorting.
export const TEST_VERDICT_STATUS_CMP_STRING = {
  [TestVerdictStatus.TEST_VERDICT_STATUS_UNSPECIFIED]: '0',
  [TestVerdictStatus.UNEXPECTED]: '1',
  [TestVerdictStatus.UNEXPECTEDLY_SKIPPED]: '2',
  [TestVerdictStatus.FLAKY]: '3',
  [TestVerdictStatus.EXONERATED]: '4',
  [TestVerdictStatus.EXPECTED]: '5',
};
/**
 * Create a test variant compare function for the given sorting key list.
 *
 * A sorting key must be one of the following:
 * 1. '{property_key}': sort by property_key in ascending order.
 * 2. '-{property_key}': sort by property_key in descending order.
 */
export function createTVCmpFn(
  sortingKeys: readonly string[],
): (v1: TestVerdictBundle, v2: TestVerdictBundle) => number {
  const sorters: Array<
    [number, (v: TestVerdictBundle) => { toString(): string }]
  > = sortingKeys.map((key) => {
    const [mul, propKey] = key.startsWith('-') ? [-1, key.slice(1)] : [1, key];
    const propGetter = createTVPropGetter(propKey);

    // Status should be be sorted by their significance not by their string
    // representation.
    if (propKey.toLowerCase() === 'status') {
      return [
        mul,
        (v) =>
          TEST_VERDICT_STATUS_CMP_STRING[propGetter(v) as TestVerdictStatus],
      ];
    }
    return [mul, propGetter];
  });
  return (v1, v2) => {
    for (const [mul, propGetter] of sorters) {
      const cmp =
        propGetter(v1).toString().localeCompare(propGetter(v2).toString()) *
        mul;
      if (cmp !== 0) {
        return cmp;
      }
    }
    return 0;
  };
}

/**
 * Create a test verdict property getter for the given property key.
 *
 * A property key must be one of the following:
 * 1. 'status': status of the test verdict.
 * 2. 'partitionTime': partition time of the test verdict.
 * 3. 'v.{variant_key}': def[variant_key] of associated variant of the test
 * verdict (e.g. v.gpu).
 */
export function createTVPropGetter(
  propKey: string,
): (v: TestVerdictBundle) => ToString {
  if (propKey.match(/^v[.]/i)) {
    const variantKey = propKey.slice(2);
    return ({ variant }) => variant.def[variantKey] || '';
  }
  propKey = propKey.toLowerCase();
  switch (propKey) {
    case 'status':
      return ({ verdict }) => verdict.status;
    case 'patitiontime':
      return ({ verdict }) => verdict.partitionTime;
    default:
      logging.warn('invalid property key', propKey);
      return () => '';
  }
}
