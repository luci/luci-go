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

import { autorun, comparer, computed, observable } from 'mobx';
import { fromPromise, IPromiseBasedObservable } from 'mobx-utils';

import { createContextLink } from '../libs/context';
import { parseSearchQuery } from '../libs/search_query';
import { InnerTag, TAG_SOURCE } from '../libs/tag';
import { unwrapObservable } from '../libs/unwrap_observable';
import { TestLoader } from '../models/test_loader';
import { TestPresentationConfig } from '../services/buildbucket';
import { createTVCmpFn, createTVPropGetter, Invocation, TestVariant } from '../services/resultdb';
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
export class InvocationState {
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
  @observable.ref columnsParam?: string[];
  @computed({ equals: comparer.shallow }) get defaultColumns() {
    let columns = this.presentationConfig.column_keys || [];
    if (this.testLoader?.hasFailureReasons) {
      columns = columns.concat('failure_reasons');
    }
    return columns;
  }
  @computed({ equals: comparer.shallow }) get displayedColumns() {
    return this.columnsParam || this.defaultColumns;
  }
  @computed get displayedColumnGetters() {
    return this.displayedColumns.map((col) => createTVPropGetter(col));
  }

  @observable customColumnWidths: { readonly [key: string]: number } = {};
  @computed get columnWidths() {
    return this.displayedColumns.map((col) => this.customColumnWidths[col] ?? 100);
  }

  @observable.ref sortingKeysParam?: string[];
  @computed({ equals: comparer.shallow }) get defaultSortingKeys() {
    return ['status', ...this.defaultColumns, 'name'];
  }
  @computed({ equals: comparer.shallow }) get sortingKeys() {
    return this.sortingKeysParam || this.defaultSortingKeys;
  }
  @computed get testVariantCmpFn(): (v1: TestVariant, v2: TestVariant) => number {
    return createTVCmpFn(this.sortingKeys);
  }

  @observable.ref groupingKeysParam?: string[];
  @computed({ equals: comparer.shallow }) get defaultGroupingKeys() {
    return this.presentationConfig.grouping_keys || ['status'];
  }
  @computed({ equals: comparer.shallow }) get groupingKeys() {
    return this.groupingKeysParam || this.defaultGroupingKeys;
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
        this.testLoader.cmpFn = this.testVariantCmpFn;
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
}

export const [provideInvocationState, consumeInvocationState] = createContextLink<InvocationState>();
