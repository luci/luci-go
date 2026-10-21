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

import { useCallback, useMemo } from 'react';

import { normalizeFilterKey } from '@/fleet/components/filters/normalize_filter_key';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import {
  constructFiltersFromAIP160,
  FilterCategory,
  useFilters,
  useFilterState,
} from '@/fleet/components/filters/use_filters';
import { FILTERS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { getFieldDefinition } from '@/fleet/pages/device_list_page/chromeos/chromeos_fields';
import { useChromeOSFields } from '@/fleet/pages/device_list_page/chromeos/use_chromeos_available_columns';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { HEALTH_FILTER_CONFIGS, HealthFilterKey } from './filter_constants';

export { HEALTH_FILTER_CONFIGS, type HealthFilterKey };

export const useHealthFilterBuilders = (
  currentFiltersAIP160?: string | null,
): {
  filterBuilders: Record<HealthFilterKey, StringListFilterCategoryBuilder>;
  isLoading: boolean;
} => {
  const { availableFields, getValues, isLoading } = useChromeOSFields();
  const [searchParams] = useSyncedSearchParams();
  const rawFilters =
    currentFiltersAIP160 !== undefined
      ? currentFiltersAIP160
      : searchParams.get(FILTERS_PARAM_KEY);

  const filterBuilders = useMemo(() => {
    const builders: Record<string, StringListFilterCategoryBuilder> = {};

    // 1. Core health filters (Model, Pool)
    for (const config of Object.values(HEALTH_FILTER_CONFIGS)) {
      const rawValues = getValues(config.dimensionSource);

      builders[config.key] = new StringListFilterCategoryBuilder()
        .setLabel(config.label)
        .setOptions(rawValues.map((v) => ({ label: v, value: v })))
        .setIsLoading(isLoading);
    }

    // 2. Dynamic labels from ChromeOS device dimensions
    const supersededLabelKeys = new Set(
      Object.values(HEALTH_FILTER_CONFIGS).flatMap((c) =>
        c.supersededLabelKeys.map((k) => k.toLowerCase()),
      ),
    );

    availableFields.forEach((def) => {
      // Exclude base and special fields (e.g. type, id, state, realm, current_task)
      if (def.type !== 'label') {
        return;
      }
      if (
        builders[def.filterKey] ||
        supersededLabelKeys.has(def.id.toLowerCase())
      ) {
        return;
      }

      const values = getValues(def.id);
      if (values.length === 0 && !isLoading) {
        return;
      }

      builders[def.filterKey] = new StringListFilterCategoryBuilder()
        .setLabel(def.header)
        .setOptions(values.map((v) => ({ label: v, value: v })))
        .setIsLoading(isLoading);
    });

    // 3. While dimensions are loading, ensure any filter keys currently in the URL have a builder
    // so they are immediately parsed and active without waiting 10s for dimensions.
    if (isLoading && rawFilters) {
      const parsed = constructFiltersFromAIP160(rawFilters);
      if (!parsed.isError) {
        for (const rawKey of Object.keys(parsed.terms)) {
          const matchKey = normalizeFilterKey(rawKey).toLowerCase();
          const alreadyExists = Object.keys(builders).some(
            (k) => normalizeFilterKey(k).toLowerCase() === matchKey,
          );
          if (!alreadyExists) {
            const cleanKey = normalizeFilterKey(rawKey);
            const def = getFieldDefinition(cleanKey);

            builders[def.filterKey] = new StringListFilterCategoryBuilder()
              .setLabel(def.header)
              .setOptions([])
              .setIsLoading(true);
          }
        }
      }
    }

    return builders;
  }, [availableFields, getValues, isLoading, rawFilters]);

  return useMemo(
    () => ({
      filterBuilders,
      isLoading,
    }),
    [filterBuilders, isLoading],
  );
};

export const useHealthFilters = (
  onApply?: (searchParams: URLSearchParams) => URLSearchParams | void,
): {
  filterValues: Record<HealthFilterKey, FilterCategory> | undefined;
  aip160: () => string;
  isLoading: boolean;
  warnings: string[];
  setFiltersBatch: (updates: Record<string, string[]>) => void;
} => {
  const { filterBuilders, isLoading } = useHealthFilterBuilders();
  const { trackEvent } = useGoogleAnalytics();

  const onApplyFilter = useCallback(
    (searchParams: URLSearchParams) => {
      trackEvent('filter_changed', {
        componentName: 'health_dashboard_filter',
      });
      return onApply?.(searchParams) ?? searchParams;
    },
    [onApply, trackEvent],
  );

  const { filterValues, aip160, warnings, setFiltersBatch } = useFilters(
    filterBuilders,
    {
      areFilterValuesLoading: isLoading,
      onFilterChange: onApplyFilter,
    },
  );

  return useMemo(
    () => ({
      filterValues: filterValues as
        | Record<HealthFilterKey, FilterCategory>
        | undefined,
      aip160,
      isLoading,
      warnings,
      setFiltersBatch,
    }),
    [filterValues, aip160, isLoading, warnings, setFiltersBatch],
  );
};

export const useHealthFilterState = (
  initialAip160?: string | null,
  onFilterChange?: (nextAip160: string) => void,
): {
  filterValues: Record<string, FilterCategory> | undefined;
  filterCategoryDatas: FilterCategory[];
  aip160: () => string;
  isLoading: boolean;
  warnings: string[];
  setFiltersBatch: (updates: Record<string, string[]>) => void;
} => {
  const { filterBuilders, isLoading } = useHealthFilterBuilders(initialAip160);
  const filterState = useFilterState(
    filterBuilders,
    initialAip160 ?? null,
    onFilterChange,
    {
      areFilterValuesLoading: isLoading,
    },
  );

  const filterCategoryDatas = useMemo(
    () =>
      filterState.filterValues ? Object.values(filterState.filterValues) : [],
    [filterState.filterValues],
  );

  return useMemo(
    () => ({
      filterValues: filterState.filterValues as
        | Record<string, FilterCategory>
        | undefined,
      filterCategoryDatas,
      aip160: filterState.aip160,
      isLoading,
      warnings: filterState.warnings,
      setFiltersBatch: filterState.setFiltersBatch,
    }),
    [filterState, filterCategoryDatas, isLoading],
  );
};
