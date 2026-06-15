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

import { useMemo, useCallback } from 'react';

import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import {
  useFilters,
  FilterCategory,
} from '@/fleet/components/filters/use_filters';
import { BROWSER_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import {
  BROWSER_SWARMING_SOURCE,
  BROWSER_UFS_SOURCE,
} from '@/fleet/constants/browser';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { getDisplayName } from './alias';
import { getBrowserColumn, getBrowserColumnIds } from './browser_columns';
import { useBrowserDeviceDimensions } from './use_browser_device_dimensions';

export const useBrowserFilters = (
  onApply?: () => void,
): {
  filterValues: Record<string, FilterCategory> | undefined;
  aip160: () => string;
  isLoading: boolean;
  warnings: string[];
} => {
  const { trackEvent } = useGoogleAnalytics();
  const [searchParams] = useSyncedSearchParams();
  const dimensionsQuery = useBrowserDeviceDimensions();

  const onApplyFilter = useCallback(() => {
    trackEvent('filter_changed', {
      componentName: 'device_list_filter',
    });
    onApply?.();
  }, [onApply, trackEvent]);

  const isDimensionsQueryProperlyLoaded =
    dimensionsQuery.data &&
    dimensionsQuery.data.baseDimensions &&
    dimensionsQuery.data.swarmingLabels &&
    dimensionsQuery.data.ufsLabels;

  const columnsRecord = useMemo(() => {
    if (!isDimensionsQueryProperlyLoaded) return {};
    const columnsParamStr = searchParams.getAll(COLUMNS_PARAM_KEY).join(',');
    const urlCols = columnsParamStr ? columnsParamStr.split(',') : [];
    const requiredCols = urlCols.length > 0 ? urlCols : BROWSER_DEFAULT_COLUMNS;
    const ids = getBrowserColumnIds(dimensionsQuery.data, requiredCols);
    return Object.fromEntries(ids.map((id) => [id, getBrowserColumn(id)]));
  }, [isDimensionsQueryProperlyLoaded, dimensionsQuery.data, searchParams]);

  const filterOptions = useMemo(() => {
    if (!isDimensionsQueryProperlyLoaded) return {};

    const filters: Record<string, StringListFilterCategoryBuilder> = {};

    const addDimensions = (
      dimensions: Record<string, { values: readonly string[] }>,
      getColumnId: (k: string) => string,
    ) => {
      for (const [filterKey, filterValues] of Object.entries(dimensions)) {
        const key = getColumnId(filterKey);
        const label = (columnsRecord[key]?.header || key) as string;
        const value = columnsRecord[key]?.filterKey || key;

        filters[value] = new StringListFilterCategoryBuilder()
          .setLabel(label)
          .setOptions([
            { label: BLANK_VALUE, value: BLANK_VALUE },
            ...(filterValues.values || [])
              .filter((v) => v !== '' && v !== BLANK_VALUE)
              .map((v) => ({
                label: getDisplayName(v, filterKey),
                value: v,
              })),
          ]);
      }
    };

    addDimensions(dimensionsQuery.data.baseDimensions, (k) => k);
    addDimensions(
      dimensionsQuery.data.swarmingLabels,
      (k) => `${BROWSER_SWARMING_SOURCE}.${k}`,
    );
    addDimensions(
      dimensionsQuery.data.ufsLabels,
      (k) => `${BROWSER_UFS_SOURCE}.${k}`,
    );

    return filters;
  }, [isDimensionsQueryProperlyLoaded, dimensionsQuery.data, columnsRecord]);

  const filterCategoryDatas = useFilters(filterOptions, {
    areFilterValuesLoading: !isDimensionsQueryProperlyLoaded,
    onFilterChange: onApplyFilter,
  });

  return {
    filterValues: filterCategoryDatas.filterValues,
    aip160: filterCategoryDatas.aip160,
    warnings: filterCategoryDatas.warnings,
    isLoading: !isDimensionsQueryProperlyLoaded,
  };
};
