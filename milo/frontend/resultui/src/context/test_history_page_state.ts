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

import { interpolateOranges, scaleLinear, scaleSequential } from 'd3';
import { DateTime } from 'luxon';
import { autorun, comparer, computed, observable, reaction } from 'mobx';

import { createContextLink } from '../libs/context';
import { parseVariantFilter } from '../libs/queries/th_filter_query';
import { TestHistoryEntriesLoader } from '../models/test_history_entries_loader';
import { TestHistoryLoader } from '../models/test_history_loader';
import { TestVariantTableState, VariantGroup } from '../pages/test_results_tab/test_variants_table/context';
import { createTVCmpFn, getCriticalVariantKeys, ResultDb, Variant } from '../services/resultdb';
import { TestHistoryService, TestVerdict } from '../services/weetbix';

export const enum GraphType {
  STATUS = 'STATUS',
  DURATION = 'DURATION',
}

export const enum XAxisType {
  DATE = 'DATE',
  COMMIT = 'COMMIT',
}

// Use SCALE_COLOR to discard colors avoid using white color when the input is
// close to 0.
const SCALE_COLOR = scaleLinear().range([0.1, 1]).domain([0, 1]);

/**
 * Records the test history page state.
 */
export class TestHistoryPageState implements TestVariantTableState {
  readonly testHistoryLoader: TestHistoryLoader;
  readonly now = DateTime.now().startOf('day').plus({ hours: 12 });
  @observable.ref days = 14;

  @computed get endDate() {
    return this.now.minus({ days: this.days });
  }

  @computed get dates() {
    return Array(this.days)
      .fill(0)
      .map((_, i) => this.now.minus({ days: i }));
  }

  @observable.ref filterText = '';
  @observable.ref private variantFilter = (_v: Variant, _hash: string) => true;
  @computed get filteredVariants() {
    return this.testHistoryLoader.variants.filter(([hash, v]) => this.variantFilter(v, hash));
  }

  @observable.ref private discoverVariantReqCount = 0;
  @computed get isDiscoveringVariants() {
    return this.discoverVariantReqCount > 0;
  }
  @observable.ref private _loadedAllVariants = false;
  @computed get loadedAllVariants() {
    return this._loadedAllVariants;
  }

  @observable.ref graphType = GraphType.STATUS;
  @observable.ref xAxisType = XAxisType.DATE;

  @observable.ref countUnexpected = true;
  @observable.ref countUnexpectedlySkipped = true;
  @observable.ref countFlaky = true;

  // Keep track of the max and min duration to render the duration graph.
  @observable.ref durationInitialized = false;
  @observable.ref maxDurationMs = 100;
  @observable.ref minDurationMs = 0;

  resetDurations() {
    this.durationInitialized = false;
    this.maxDurationMs = 100;
    this.minDurationMs = 0;
  }

  @computed get scaleDurationColor() {
    return scaleSequential((x) => interpolateOranges(SCALE_COLOR(x))).domain([this.minDurationMs, this.maxDurationMs]);
  }

  @computed({ equals: comparer.shallow }) get criticalVariantKeys(): readonly string[] {
    return getCriticalVariantKeys(this.testHistoryLoader.variants.map(([_, v]) => v));
  }

  @observable.ref private customColumnKeys?: readonly string[];
  @computed get defaultColumnKeys(): readonly string[] {
    return this.criticalVariantKeys.map((k) => 'v.' + k);
  }
  setColumnKeys(v: readonly string[]): void {
    this.customColumnKeys = v;
  }
  @computed({ equals: comparer.shallow }) get columnKeys(): readonly string[] {
    return this.customColumnKeys || this.defaultColumnKeys;
  }

  @observable.ref private customColumnWidths: { readonly [key: string]: number } = {};
  @computed get columnWidths() {
    return this.columnKeys.map((col) => this.customColumnWidths[col] ?? 100);
  }
  setColumnWidths(v: { readonly [key: string]: number }): void {
    this.customColumnWidths = v;
  }

  readonly enablesGrouping = false;
  readonly defaultGroupingKeys = [];
  readonly groupingKeys: readonly string[] = [];
  setGroupingKeys() {}

  @observable.ref private customSortingKeys?: readonly string[];
  readonly defaultSortingKeys: readonly string[] = [];
  @computed get sortingKeys(): readonly string[] {
    return this.customSortingKeys || this.defaultSortingKeys;
  }
  setSortingKeys(v: readonly string[]): void {
    this.customSortingKeys = v;
  }

  @observable.ref isDisposed = false;
  private disposers: Array<() => void> = [];
  constructor(
    readonly realm: string,
    readonly testId: string,
    readonly testHistoryService: TestHistoryService,
    readonly resultDb: ResultDb
  ) {
    this.testHistoryLoader = new TestHistoryLoader(
      realm,
      testId,
      (datetime) => datetime.toFormat('yyyy-MM-dd'),
      testHistoryService,
      resultDb
    );
    this.discoverVariants();

    // Ensure the first page of test history entry details are loaded / being
    // loaded.
    this.disposers.push(
      reaction(
        () => this.entriesLoader,
        () => this.entriesLoader?.loadFirstPage(),
        { fireImmediately: true }
      )
    );

    // Keep the filters in sync.
    this.disposers.push(
      autorun(() => {
        try {
          const newVariantFilter = parseVariantFilter(this.filterText);

          // Only update the filters after the query is successfully parsed.
          this.variantFilter = newVariantFilter;
        } catch (e) {
          //TODO(weiweilin): display the error to the user.
          console.error(e);
        }
      })
    );
  }

  async discoverVariants() {
    this.discoverVariantReqCount++;
    const req = this.testHistoryLoader.discoverVariants();
    req.finally(() => this.discoverVariantReqCount--);
    this._loadedAllVariants = await req;
    return this._loadedAllVariants;
  }

  @computed({ keepAlive: true })
  private get entriesLoader() {
    if (this.isDisposed) {
      return null;
    }
    return new TestHistoryEntriesLoader(this.testId, this.selectedTvhEntries, this.resultDb);
  }

  @observable.ref selectedTvhEntries: readonly TestVerdict[] = [];

  @computed get variantGroups(): readonly VariantGroup[] {
    if (this.entriesLoader!.testVariants.length === 0) {
      return [];
    }

    const cmpFn = createTVCmpFn(this.sortingKeys);
    return [
      {
        def: [],
        variants: [...this.entriesLoader!.testVariants].sort(cmpFn),
      },
    ];
  }

  @computed
  get testVariantCount() {
    return this.entriesLoader!.testVariants.length;
  }
  @computed get unfilteredTestVariantCount() {
    return this.entriesLoader!.testVariants.length;
  }

  readonly readyToLoad = true;
  @computed get isLoading() {
    return this.entriesLoader!.isLoading;
  }
  get loadedAllTestVariants() {
    return this.entriesLoader!.loadedAllTestVariants;
  }
  get loadedFirstPage() {
    return this.entriesLoader!.loadedFirstPage;
  }

  loadFirstPage() {
    return this.entriesLoader!.loadFirstPage();
  }
  loadNextPage() {
    return this.entriesLoader!.loadNextPage();
  }

  // Don't display history URL when the user is already on the history page.
  getHistoryUrl() {
    return '';
  }

  /**
   * Perform cleanup.
   * Must be called before the object is GCed.
   */
  dispose() {
    this.isDisposed = true;
    for (const disposer of this.disposers) {
      disposer();
    }

    // Evaluates @computed({keepAlive: true}) properties after this.isDisposed
    // is set to true so they no longer subscribes to any external observable.
    this.entriesLoader;
  }
}

export const [provideTestHistoryPageState, consumeTestHistoryPageState] = createContextLink<TestHistoryPageState>();
