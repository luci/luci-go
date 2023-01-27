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

import { deepEqual } from 'fast-equals';
import { html } from 'lit';
import { autorun, comparer, computed } from 'mobx';
import { addDisposer, Instance, SnapshotIn, SnapshotOut, types } from 'mobx-state-tree';
import { fromPromise } from 'mobx-utils';

import { NEVER_OBSERVABLE, NEVER_PROMISE } from '../libs/constants';
import { createContextLink } from '../libs/context';
import { keepAliveComputed, unwrapObservable } from '../libs/milo_mobx_utils';
import { parseTestResultSearchQuery, TestVariantFilter } from '../libs/queries/tr_search_query';
import { InnerTag, TAG_SOURCE } from '../libs/tag';
import { TestLoader } from '../models/test_loader';
import { VariantGroup } from '../pages/test_results_tab/test_variants_table';
import { router } from '../routes';
import { TestPresentationConfig } from '../services/buildbucket';
import { createTVCmpFn, createTVPropGetter, RESULT_LIMIT, TestVariant, TestVariantStatus } from '../services/resultdb';
import { ServicesStore } from './services';

export class QueryInvocationError extends Error implements InnerTag {
  readonly [TAG_SOURCE]: Error;

  constructor(readonly invId: string, readonly source: Error) {
    super(source.message);
    this[TAG_SOURCE] = source;
  }
}

export const InvocationState = types
  .model('InvocationState', {
    services: types.safeReference(ServicesStore),

    searchText: '',
    customColumnKeys: types.frozen<readonly string[]>([]),
    customColumnWidths: types.frozen<{ readonly [key: string]: number }>({}),
    customSortingKeys: types.frozen<readonly string[]>([]),
    customGroupingKeys: types.frozen<readonly string[]>([]),
  })
  .volatile(() => ({
    // Use getters instead of plain values so the values can be derived from
    // the parent node (or whoever set the dependencies).
    invocationIdGetter: () => null as string | null,
    presentationConfigGetter: () => ({} as TestPresentationConfig),
    warningGetter: () => '' as string,

    searchFilter: ((_v: TestVariant) => true) as TestVariantFilter,
  }))
  .views((self) => ({
    get invocationId() {
      return self.invocationIdGetter();
    },
    get presentationConfig() {
      return self.presentationConfigGetter();
    },
    get warning() {
      return self.warningGetter();
    },
  }))
  .views((self) => {
    const defaultColumnKeys = computed(() => self.presentationConfig.column_keys || [], { equals: comparer.shallow });
    const columnKeys = computed(() => self.customColumnKeys || defaultColumnKeys.get(), { equals: comparer.shallow });
    const defaultSortingKeys = computed(() => ['status', ...defaultColumnKeys.get(), 'name'], {
      equals: comparer.shallow,
    });
    const sortingKeys = computed(() => self.customSortingKeys || defaultSortingKeys.get(), {
      equals: comparer.shallow,
    });
    const defaultGroupingKeys = computed(() => self.presentationConfig.grouping_keys || ['status'], {
      equals: comparer.shallow,
    });
    const groupingKeys = computed(() => self.customGroupingKeys || defaultGroupingKeys.get(), {
      equals: comparer.shallow,
    });

    return {
      get defaultColumnKeys() {
        return defaultColumnKeys.get();
      },
      get columnKeys() {
        return columnKeys.get();
      },
      get columnGetters() {
        return this.columnKeys.map((col) => createTVPropGetter(col));
      },
      get defaultSortingKeys() {
        return defaultSortingKeys.get();
      },
      get sortingKeys() {
        return sortingKeys.get();
      },
      get defaultGroupingKeys() {
        return defaultGroupingKeys.get();
      },
      get groupingKeys() {
        return groupingKeys.get();
      },
      get columnWidths() {
        return this.columnKeys.map((col) => self.customColumnWidths[col] ?? 100);
      },
      get groupers() {
        return this.groupingKeys.map((key) => [key, createTVPropGetter(key)] as const);
      },
      get invocationName() {
        if (!self.invocationId) {
          return null;
        }
        return 'invocations/' + self.invocationId;
      },
    };
  })
  .views((self) => {
    const invocation = keepAliveComputed(self, () => {
      if (!self.services?.resultDb || !self.invocationName) {
        return null;
      }
      const invId = self.invocationId;
      return fromPromise(
        self.services.resultDb.getInvocation({ name: self.invocationName }).catch((e) => {
          throw new QueryInvocationError(invId!, e);
        })
      );
    });

    const testLoader = keepAliveComputed(self, () => {
      if (!self.invocationName || !self.services?.resultDb) {
        return null;
      }
      return new TestLoader({ invocations: [self.invocationName], resultLimit: RESULT_LIMIT }, self.services.resultDb);
    });

    return {
      get invocation() {
        return unwrapObservable(invocation.get() || NEVER_OBSERVABLE, null);
      },
      get testLoader() {
        return testLoader.get();
      },
      get project() {
        return this.invocation?.realm.split(':', 2)[0] ?? null;
      },
      get variantGroups() {
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
            def: self.groupers.map(([key, getter]) => [key, getter(group[0])] as [string, unknown]),
            variants: group,
          })),
          {
            def: [['status', TestVariantStatus.EXPECTED]],
            variants: this.testLoader.expectedTestVariants,
            note: deepEqual(self.groupingKeys, ['status'])
              ? ''
              : html`<b>note: custom grouping doesn't apply to expected tests</b>`,
          }
        );
        return ret;
      },
      get testVariantCount() {
        return this.testLoader?.testVariantCount || 0;
      },
      get unfilteredTestVariantCount() {
        return this.testLoader?.unfilteredTestVariantCount || 0;
      },
      get loadedAllTestVariants() {
        return this.testLoader?.loadedAllVariants || false;
      },
      get readyToLoad() {
        return Boolean(this.testLoader);
      },
      get isLoading() {
        return this.testLoader?.isLoading || false;
      },
      get loadedFirstPage() {
        return this.testLoader?.firstPageLoaded || false;
      },
      getHistoryUrl(testId: string, variantHash: string) {
        if (!this.invocation?.realm) {
          return '';
        }
        const searchParam = new URLSearchParams({
          q: 'VHASH:' + variantHash,
        });
        const project = this.invocation.realm.split(':', 2)[0];
        return router.urlForName('test-history', { realm: project, test_id: testId }) + '?' + searchParam.toString();
      },
    };
  })
  .actions((self) => ({
    setDependencies(
      deps: Partial<Pick<typeof self, 'services' | 'invocationIdGetter' | 'presentationConfigGetter' | 'warningGetter'>>
    ) {
      Object.assign<typeof self, Partial<typeof self>>(self, deps);
    },
    setSearchText(v: string) {
      self.searchText = v;
    },
    setColumnKeys(v: readonly string[]) {
      self.customColumnKeys = v;
    },
    setColumnWidths(v: { readonly [key: string]: number }): void {
      self.customColumnWidths = v;
    },
    setSortingKeys(v: readonly string[]): void {
      self.customSortingKeys = v;
    },
    setGroupingKeys(v: readonly string[]): void {
      self.customGroupingKeys = v;
    },
    _setSearchFilter(v: TestVariantFilter) {
      self.searchFilter = v;
    },
    loadFirstPage() {
      return self.testLoader?.loadFirstPageOfTestVariants() || NEVER_PROMISE;
    },
    loadNextPage() {
      return self.testLoader?.loadNextTestVariants() || NEVER_PROMISE;
    },
    afterCreate() {
      addDisposer(
        self,
        autorun(() => {
          try {
            this._setSearchFilter(parseTestResultSearchQuery(self.searchText));
          } catch (e) {
            //TODO(weiweilin): display the error to the user.
            console.error(e);
          }
        })
      );

      addDisposer(
        self,
        autorun(() => {
          if (!self.testLoader) {
            return;
          }
          self.testLoader.filter = self.searchFilter;
          self.testLoader.groupers = self.groupers;
          self.testLoader.cmpFn = createTVCmpFn(self.sortingKeys);
        })
      );
    },
  }));

export type InvocationStateInstance = Instance<typeof InvocationState>;
export type InvocationStateSnapshotIn = SnapshotIn<typeof InvocationState>;
export type InvocationStateSnapshotOut = SnapshotOut<typeof InvocationState>;

export const [provideInvocationState, consumeInvocationState] = createContextLink<InvocationStateInstance>();
