// Copyright 2026 The LUCI Authors.
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

import { useMemo } from 'react';

import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { FilterCategory } from '@/fleet/components/filters/use_filters';

import { useHealthFilterState } from './use_health_filters';

export interface HealthFilterBarProps {
  /** Optional pre-built categories (e.g. from page-level useHealthFilters). */
  readonly filterCategoryDatas?: FilterCategory[];
  /** Controlled AIP-160 filter string for local/dialog filter bars. */
  readonly aip160?: string;
  readonly onFilterChange?: (nextAip160: string) => void;
  readonly isLoading?: boolean;
  readonly searchPlaceholder?: string;
  readonly disableShortcut?: boolean;
  readonly onApply?: () => void;
}

/**
 * HealthFilterBar encapsulates the Unified FilterBar for ChromeOS Health dashboard views.
 * It can either receive external filterCategoryDatas (e.g. from page-level useHealthFilters)
 * or manage its own isolated filter state via useHealthFilterState when given an aip160 string.
 *
 * Since both modes share useHealthFilterBuilders, any new filter category added to the health
 * dashboard is automatically available both in the global filter bar and in health slices.
 */
export const HealthFilterBar = ({
  filterCategoryDatas: explicitFilterCategoryDatas,
  aip160,
  onFilterChange,
  isLoading: explicitIsLoading,
  searchPlaceholder = 'Add a filter (e.g. model:volteer)',
  disableShortcut = false,
  onApply,
}: HealthFilterBarProps) => {
  const isControlledByAip160 = explicitFilterCategoryDatas === undefined;
  const { filterCategoryDatas: stateCategoryDatas, isLoading: stateLoading } =
    useHealthFilterState(
      isControlledByAip160 ? aip160 : undefined,
      isControlledByAip160 ? onFilterChange : undefined,
    );

  const filterCategoryDatas = useMemo(() => {
    if (explicitFilterCategoryDatas !== undefined) {
      return explicitFilterCategoryDatas;
    }
    return stateCategoryDatas;
  }, [explicitFilterCategoryDatas, stateCategoryDatas]);

  const isLoading =
    explicitIsLoading ?? (isControlledByAip160 ? stateLoading : false);

  return (
    <FilterBar
      filterCategoryDatas={filterCategoryDatas}
      isLoading={isLoading}
      searchPlaceholder={searchPlaceholder}
      disableShortcut={disableShortcut}
      onApply={onApply}
    />
  );
};
