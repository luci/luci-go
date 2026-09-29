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
import { useDeviceDimensions } from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import {
  GetDeviceDimensionsResponse,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { HEALTH_FILTER_CONFIGS, HealthFilterKey } from './filter_constants';

export { HEALTH_FILTER_CONFIGS, type HealthFilterKey };

const getDimensionValues = (
  dimensions: GetDeviceDimensionsResponse,
  dimensionSources: readonly string[],
): readonly string[] => {
  for (const key of dimensionSources) {
    const values =
      dimensions.baseDimensions?.[key]?.values ||
      dimensions.labels?.[key]?.values;
    if (values && values.length > 0) {
      return values;
    }
  }
  return [];
};

export const useHealthFilterBuilders = (): {
  filterBuilders:
    | Record<HealthFilterKey, StringListFilterCategoryBuilder>
    | undefined;
  isLoading: boolean;
} => {
  const dimensionsQuery = useDeviceDimensions({ platform: Platform.CHROMEOS });

  const filterBuilders = useMemo(() => {
    if (dimensionsQuery.isPending || !dimensionsQuery.data) {
      return undefined;
    }

    const builders = {} as Record<
      HealthFilterKey,
      StringListFilterCategoryBuilder
    >;

    for (const config of Object.values(HEALTH_FILTER_CONFIGS)) {
      const rawValues = getDimensionValues(
        dimensionsQuery.data,
        config.dimensionSources,
      );

      builders[config.key] = new StringListFilterCategoryBuilder()
        .setLabel(config.label)
        .setOptions(rawValues.map((v) => ({ label: v, value: v })));
    }

    return builders;
  }, [dimensionsQuery.data, dimensionsQuery.isPending]);

  return useMemo(
    () => ({
      filterBuilders,
      isLoading: dimensionsQuery.isPending,
    }),
    [filterBuilders, dimensionsQuery.isPending],
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
