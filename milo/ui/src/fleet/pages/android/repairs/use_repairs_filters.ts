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

import { useQuery } from '@tanstack/react-query';
import _ from 'lodash';
import { useCallback, useMemo } from 'react';

import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { useFilters } from '@/fleet/components/filters/use_filters';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export const useRepairsFilters = (onFilterChange: () => void) => {
  const { trackEvent } = useGoogleAnalytics();
  const client = useFleetConsoleClient();

  const repairMetricsFilterValues = useQuery({
    ...client.GetRepairMetricsDimensions.query({
      platform: Platform.ANDROID,
    }),
  });

  const loadedFilterOptions = useMemo(() => {
    if (!repairMetricsFilterValues.data) return {};

    const filters: Record<string, StringListFilterCategoryBuilder> = {};
    for (const [key, value] of Object.entries(
      repairMetricsFilterValues.data.dimensions,
    )) {
      const mappedKey =
        key === 'hostGroup'
          ? 'host_group'
          : key === 'labName'
            ? 'lab_name'
            : key;
      filters[mappedKey] = new StringListFilterCategoryBuilder()
        .setLabel(_.startCase(key))
        .setOptions([
          { label: BLANK_VALUE, value: BLANK_VALUE },
          ...(value.values || [])
            .filter((v) => v !== '' && v !== BLANK_VALUE)
            .map((v) => ({ label: v, value: v })),
        ]);
    }
    return filters;
  }, [repairMetricsFilterValues.data]);

  const onApplyFilter = useCallback(() => {
    trackEvent('filter_changed', {
      componentName: 'device_list_filter',
    });
    onFilterChange();
  }, [onFilterChange, trackEvent]);

  const filterCategoryDatas = useFilters(loadedFilterOptions, {
    areFilterValuesLoading: !repairMetricsFilterValues.data,
    onFilterChange: onApplyFilter,
  });

  return {
    filterValues: filterCategoryDatas.filterValues,
    aip160: filterCategoryDatas.aip160,
    warnings: filterCategoryDatas.warnings,
    isLoading: !repairMetricsFilterValues.data,
    setFiltersBatch: filterCategoryDatas.setFiltersBatch,
  };
};
