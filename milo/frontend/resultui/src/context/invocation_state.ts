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

import { deepEqual } from 'fast-equals';
import { html } from 'lit-html';
import { autorun, comparer, computed, observable } from 'mobx';
import { fromPromise, IPromiseBasedObservable } from 'mobx-utils';

import { TestVariantTableState, VariantGroup } from '../components/test_variants_table/context';
import { createContextLink } from '../libs/context';
import { parseSearchQuery } from '../libs/search_query';
import { InnerTag, TAG_SOURCE } from '../libs/tag';
import { unwrapObservable } from '../libs/unwrap_observable';
import { TestLoader } from '../models/test_loader';
import { router } from '../routes';
import { TestPresentationConfig } from '../services/buildbucket';
import { createTVCmpFn, createTVPropGetter, Invocation, TestVariant, TestVariantStatus } from '../services/resultdb';
import { AppState } from './app_state';

export class QueryInvocationError extends Error implements InnerTag {
  readonly [TAG_SOURCE]: Error;

  constructor(readonly invId: string, readonly source: Error) {
    super(source.message);
    this[TAG_SOURCE] = source;
  }
}

/**
 * Records state of an invocation.
 */
export class InvocationState implements TestVariantTableState {
  // '' means no associated invocation ID.
  // null means uninitialized.
  @observable.ref invocationId: string | null = null;

  // Whether the invocation ID is computed.
  // A matching invocation may not exist for a computed invocation ID.
  @observable.ref isComputedInvId = false;

  @observable.ref warning = '';

  @observable.ref searchText = '';
  @observable.ref searchFilter = (_v: TestVariant) => true;

  @observable.ref presentationConfig: TestPresentationConfig = {};

  getHistoryUrl(testId: string) {
    if (!this.invocation?.realm) {
      return '';
    }
    return router.urlForName('test-history', { realm: this.invocation.realm, test_id: testId });
  }

  @observable.ref private customColumnKeys?: readonly string[];
  @computed({ equals: comparer.shallow }) get defaultColumnKeys() {
    return this.presentationConfig.column_keys || [];
  }
  @computed({ equals: comparer.shallow }) get columnKeys() {
    return this.customColumnKeys || this.defaultColumnKeys;
  }
  setColumnKeys(v: readonly string[]): void {
    this.customColumnKeys = v;
  }

  @observable.ref private customColumnWidths: { readonly [key: string]: number } = {};
  setColumnWidths(v: { readonly [key: string]: number }): void {
    this.customColumnWidths = v;
  }
  @computed get columnWidths() {
    return this.columnKeys.map((col) => this.customColumnWidths[col] ?? 100);
  }

  @observable.ref private customSortingKeys?: readonly string[];
  setSortingKeys(v: readonly string[]): void {
    this.customSortingKeys = v;
  }
  @computed({ equals: comparer.shallow }) get defaultSortingKeys() {
    return ['status', ...this.defaultColumnKeys, 'name'];
  }
  @computed({ equals: comparer.shallow }) get sortingKeys() {
    return this.customSortingKeys || this.defaultSortingKeys;
  }

  readonly enablesGrouping = true;
  @observable.ref private customGroupingKeys?: readonly string[];
  setGroupingKeys(v: readonly string[]): void {
    this.customGroupingKeys = v;
  }
  @computed({ equals: comparer.shallow }) get defaultGroupingKeys() {
    return this.presentationConfig.grouping_keys || ['status'];
  }
  @computed({ equals: comparer.shallow }) get groupingKeys() {
    return this.customGroupingKeys || this.defaultGroupingKeys;
  }
  @computed get groupers(): Array<[string, (v: TestVariant) => unknown]> {
    return this.groupingKeys.map((key) => [key, createTVPropGetter(key)]);
  }

  private disposers: Array<() => void> = [];
  constructor(private appState: AppState) {
    this.disposers.push(
      autorun(() => {
        try {
          this.searchFilter = parseSearchQuery(this.searchText);
        } catch (e) {
          //TODO(weiweilin): display the error to the user.
          console.error(e);
        }
      })
    );
    this.disposers.push(
      autorun(() => {
        if (!this.testLoader) {
          return;
        }
        this.testLoader.filter = this.searchFilter;
        this.testLoader.groupers = this.groupers;
        this.testLoader.cmpFn = createTVCmpFn(this.sortingKeys);
      })
    );
  }

  @observable.ref private isDisposed = false;

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
    this.testLoader;
  }

  @computed
  get invocationName(): string | null {
    if (!this.invocationId) {
      return null;
    }
    return 'invocations/' + this.invocationId;
  }

  @computed
  private get invocation$(): IPromiseBasedObservable<Invocation> {
    if (!this.appState.resultDb || !this.invocationName) {
      // Returns a promise that never resolves when resultDb isn't ready.
      return fromPromise(Promise.race([]));
    }
    const invId = this.invocationId;
    return fromPromise(
      this.appState.resultDb.getInvocation({ name: this.invocationName }).catch((e) => {
        throw new QueryInvocationError(invId!, e);
      })
    );
  }

  @computed
  get invocation(): Invocation | null {
    return unwrapObservable(this.invocation$, null);
  }

  @computed get hasInvocation() {
    if (this.isComputedInvId) {
      // The invocation may not exist. Wait for the invocation query to confirm
      // its existence.
      return this.invocation !== null;
    }
    return Boolean(this.invocationId);
  }

  @computed({ keepAlive: true })
  get testLoader(): TestLoader | null {
    if (this.isDisposed || !this.invocationName || !this.appState.resultDb) {
      return null;
    }
    return new TestLoader({ invocations: [this.invocationName] }, this.appState.resultDb);
  }

  @computed get variantGroups() {
    if (!this.testLoader) {
      return [];
    }
    const ret: VariantGroup[] = [];
    if (this.testLoader.loadedAllUnexpectedVariants && this.testLoader.unexpectedTestVariants.length === 0) {
      // Indicates that there are no unexpected test variants.
      ret.push({
        def: [['status', TestVariantStatus.UNEXPECTED]],
        variants: [],
      });
    }
    ret.push(
      ...this.testLoader.groupedNonExpectedVariants.map((group) => ({
        def: this.groupers.map(([key, getter]) => [key, getter(group[0])] as [string, unknown]),
        variants: group,
      })),
      {
        def: [['status', TestVariantStatus.EXPECTED]],
        variants: this.testLoader.expectedTestVariants,
        note: deepEqual(this.groupingKeys, ['status'])
          ? ''
          : html`<b>note: custom grouping doesn't apply to expected tests</b>`,
      }
    );
    return ret;
  }
  @computed get testVariantCount() {
    return this.testLoader?.testVariantCount || 0;
  }
  @computed get unfilteredTestVariantCount() {
    return this.testLoader?.unfilteredTestVariantCount || 0;
  }
  @computed get loadedAllTestVariants() {
    return this.testLoader?.loadedAllVariants || false;
  }

  @computed get readyToLoad() {
    return Boolean(this.testLoader);
  }
  @computed get isLoading() {
    return this.testLoader?.isLoading || false;
  }
  @computed get loadedFirstPage() {
    return this.testLoader?.firstPageLoaded || false;
  }
  loadFirstPage(): Promise<void> {
    return this.testLoader?.loadFirstPageOfTestVariants() || Promise.race([]);
  }
  loadNextPage(): Promise<void> {
    return this.testLoader?.loadNextTestVariants() || Promise.race([]);
  }
}

export const [provideInvocationState, consumeInvocationState] = createContextLink<InvocationState>();
