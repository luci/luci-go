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

import { RangeFilterCategoryBuilder } from '@/fleet/components/filters/range_filter';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import {
  useFilters,
  FilterCategory,
  FilterCategoryBuilder,
} from '@/fleet/components/filters/use_filters';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { useDeviceDimensions } from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { ANDROID_COLUMN_OVERRIDES } from './android_fields';
import { ANDROID_EXTRA_FILTERS } from './android_filters';

export const useAndroidFilters = (
  onFilterChange: () => void,
  showAvgUtilization = false,
) => {
  const { trackEvent } = useGoogleAnalytics();
  const dimensionsQuery = useDeviceDimensions({ platform: Platform.ANDROID });

  const isDimensionsQueryProperlyLoaded =
    dimensionsQuery.data &&
    dimensionsQuery.data.baseDimensions &&
    dimensionsQuery.data.labels;

  const onFilterChangeCallback = useCallback(() => {
    trackEvent('filter_changed', {
      componentName: 'device_list_filter',
    });
    onFilterChange();
  }, [onFilterChange, trackEvent]);

  const filterOptions = useMemo(() => {
    const extraFilters: Record<
      string,
      FilterCategoryBuilder<FilterCategory>
    > = {
      ...ANDROID_EXTRA_FILTERS,
    };
    if (showAvgUtilization) {
      extraFilters['"average_7d"'] = new RangeFilterCategoryBuilder()
        .setLabel('7 Day Average Utilization')
        .setMin(0)
        .setMax(100);
      extraFilters['"average_30d"'] = new RangeFilterCategoryBuilder()
        .setLabel('30 Day Average Utilization')
        .setMin(0)
        .setMax(100);
    }
    if (!isDimensionsQueryProperlyLoaded || !dimensionsQuery.data) {
      return extraFilters;
    }

    const filters = extraFilters;
    const data = dimensionsQuery.data;
    const baseKeys = Object.keys(data.baseDimensions);

    for (const [key, value] of [
      ...Object.entries(data.baseDimensions),
      ...Object.entries(data.labels),
    ]) {
      if (value.values.length === 0) continue;

      const isBase = baseKeys.includes(key);
      const filterKey = isBase ? `"${key}"` : `labels."${key}"`;

      if (filters[filterKey]) continue;

      const override = ANDROID_COLUMN_OVERRIDES[key];
      const label = override?.header || key;

      filters[filterKey] = new StringListFilterCategoryBuilder()
        .setLabel(label)
        .setOptions([
          { label: BLANK_VALUE, value: BLANK_VALUE },
          ...value.values.map((v) => ({ label: v, value: v })),
        ]);
    }

    return filters;
  }, [
    isDimensionsQueryProperlyLoaded,
    dimensionsQuery.data,
    showAvgUtilization,
  ]);

  const filterCategoryDatas = useFilters(filterOptions, {
    areFilterValuesLoading: !isDimensionsQueryProperlyLoaded,
    onFilterChange: onFilterChangeCallback,
  });

  return {
    filterValues: filterCategoryDatas.filterValues,
    aip160: filterCategoryDatas.aip160,
    warnings: filterCategoryDatas.warnings,
    isLoading: !isDimensionsQueryProperlyLoaded,
    setFiltersBatch: filterCategoryDatas.setFiltersBatch,
  };
};
