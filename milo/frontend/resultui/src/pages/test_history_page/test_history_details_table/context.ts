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

import { createContextLink } from '../../../libs/context';
import { TestVariant } from '../../../services/resultdb';

export interface VariantGroup {
  readonly def: ReadonlyArray<readonly [string, unknown]>;
  readonly variants: readonly TestVariant[];
  readonly note?: unknown;
}

/**
 * The context state that the test variant table relies on.
 * Each property should either be immutable (i.e. the object should be re-
 * constructed when the property is updated) or be declared as observable.
 */
export interface TestVariantTableState {
  readonly defaultColumnKeys: readonly string[];
  readonly columnKeys: readonly string[];
  setColumnKeys(v: readonly string[]): void;

  readonly columnWidths: readonly number[];
  setColumnWidths(v: { readonly [key: string]: number }): void;

  readonly enablesGrouping: boolean;
  readonly defaultGroupingKeys: readonly string[];
  readonly groupingKeys: readonly string[];
  setGroupingKeys(v: readonly string[]): void;

  readonly defaultSortingKeys: readonly string[];
  readonly sortingKeys: readonly string[];
  setSortingKeys(v: readonly string[]): void;

  readonly variantGroups: readonly VariantGroup[];
  readonly testVariantCount: number;
  readonly unfilteredTestVariantCount: number;

  /**
   * Whether loadFirstPage and loadNextPage are ready to be called.
   * When false, calling loadFirstPage or loadNextPage is a no-op.
   */
  readonly readyToLoad: boolean;
  readonly isLoading: boolean;
  readonly loadedAllTestVariants: boolean;
  readonly loadedFirstPage: boolean;
  loadFirstPage(): Promise<void>;
  loadNextPage(): Promise<void>;

  getHistoryUrl(testId: string, variantHash: string): string;
}

export const [provideTestVariantTableState, consumeTestVariantTableState] = createContextLink<TestVariantTableState>();
