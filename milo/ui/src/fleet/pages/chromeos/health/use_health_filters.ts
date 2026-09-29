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

import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import {
  FilterCategory,
  useFilters,
} from '@/fleet/components/filters/use_filters';
import { useChromeOSFields } from '@/fleet/pages/device_list_page/chromeos/use_chromeos_available_columns';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';

import { HEALTH_FILTER_CONFIGS, HealthFilterKey } from './filter_constants';

export { HEALTH_FILTER_CONFIGS, type HealthFilterKey };

export const useHealthFilterBuilders = (): {
  filterBuilders:
    | Record<HealthFilterKey, StringListFilterCategoryBuilder>
    | undefined;
  isLoading: boolean;
} => {
  const { availableFields, getValues, isLoading } = useChromeOSFields();

  const filterBuilders = useMemo(() => {
    if (isLoading) {
      return undefined;
    }

    const builders: Record<string, StringListFilterCategoryBuilder> = {};

    // 1. Core health filters (Model, Pool)
    for (const config of Object.values(HEALTH_FILTER_CONFIGS)) {
      const rawValues = getValues(config.dimensionSource);

      builders[config.key] = new StringListFilterCategoryBuilder()
        .setLabel(config.label)
        .setOptions(rawValues.map((v) => ({ label: v, value: v })));
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
      if (values.length === 0) {
        return;
      }

      builders[def.filterKey] = new StringListFilterCategoryBuilder()
        .setLabel(def.header)
        .setOptions(values.map((v) => ({ label: v, value: v })));
    });

    return builders;
  }, [availableFields, getValues, isLoading]);

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
