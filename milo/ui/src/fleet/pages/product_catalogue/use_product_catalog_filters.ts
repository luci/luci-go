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

import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useState, useCallback } from 'react';

import { RangeFilterCategoryBuilder } from '@/fleet/components/filters/range_filter';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { useFilters } from '@/fleet/components/filters/use_filters';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { FILTERS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { combineAipFilters } from '@/fleet/utils/search_param';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  GetProductCatalogFilterValuesResponse,
  Int32Range,
  ProductCatalogFilterValue,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { COLUMNS } from './product_catalogue_columns';
import { ProductCatalogTab } from './use_product_catalog_tabs';

export const FILTERS = {
  productCatalogId: { type: 'string_list', filterKey: 'product_catalog_id' },
  productName: { type: 'string_list', filterKey: 'product_name' },
  gpn: { type: 'string_list', filterKey: 'gpn' },
  resourceType: { type: 'string_list', filterKey: 'resource_type' },
  fleetPlmStatus: {
    type: 'string_list',
    filterKey: 'fleet_plm_status',
  },
  r11n: { type: 'string_list', filterKey: 'r11n' },
  numberOfDevicesPerRack: {
    type: 'range',
    filterKey: 'number_of_devices_per_rack',
  },
  productType: { type: 'string_list', filterKey: 'product_type' },
} satisfies Partial<
  Record<
    keyof GetProductCatalogFilterValuesResponse,
    {
      type: 'string_list' | 'range';
      filterKey: string;
    }
  >
>;

export const DEFAULT_FILTER_VALUES: Partial<
  Record<keyof typeof FILTERS, readonly string[]>
> = {
  fleetPlmStatus: ['GA', 'LA', 'NPI'],
};

export const useProductCatalogFilters = (
  selectedTab: ProductCatalogTab,
  onApply?: () => void,
) => {
  const [searchParams] = useSyncedSearchParams();
  const hasUrlFiltersParam = searchParams.get(FILTERS_PARAM_KEY) !== null;
  const [filterOptions, setFilterOptions] = useState<
    | Record<
        string,
        StringListFilterCategoryBuilder | RangeFilterCategoryBuilder
      >
    | undefined
  >(undefined);
  const { trackEvent } = useGoogleAnalytics();

  const onFilterChange = useCallback(() => {
    trackEvent('product_catalogue_search', {
      componentName: 'product_catalogue_filter',
    });
  }, [trackEvent]);

  const isAllTab = selectedTab === ProductCatalogTab.ALL;
  const currentTab = isAllTab
    ? ''
    : `("${FILTERS.productType.filterKey}" = "${selectedTab}")`;
  //Ideally we would want to use filterValues?.[productType] to set value

  const { filterValues, aip160, warnings, setFiltersBatch } = useFilters(
    filterOptions,
    {
      onFilterChange,
    },
  );

  const combinedFilter = combineAipFilters(aip160(), currentTab);

  const client = useFleetConsoleClient();
  const filterOptionsQuery = useQuery({
    ...client.GetProductCatalogFilterValues.query({ filter: combinedFilter }),
    placeholderData: keepPreviousData,
  });

  const nextFilterOptions = useMemo(() => {
    if (!filterOptionsQuery.data) return undefined;

    const options: Record<
      string,
      StringListFilterCategoryBuilder | RangeFilterCategoryBuilder
    > = {};
    for (const column of COLUMNS) {
      if (!('accessorKey' in column) || !column.accessorKey) continue;
      if (!(column.accessorKey in FILTERS)) continue;

      const accessorKey = column.accessorKey as keyof typeof FILTERS;
      const config = FILTERS[accessorKey];

      const data = filterOptionsQuery.data?.[accessorKey];
      const filterKey = `"${config.filterKey}"`;
      const scopedKey =
        `scoped${accessorKey.charAt(0).toUpperCase()}${accessorKey.slice(1)}` as keyof GetProductCatalogFilterValuesResponse;
      const scopedData = filterOptionsQuery.data?.[scopedKey] as
        | ProductCatalogFilterValue[]
        | undefined;

      if (accessorKey === 'productType' && !isAllTab) continue;

      if (config.type === 'string_list') {
        const defaultOptions = hasUrlFiltersParam
          ? []
          : (DEFAULT_FILTER_VALUES[accessorKey] ?? []);
        options[filterKey] = new StringListFilterCategoryBuilder()
          .setLabel(column.header as string)
          .setOptions(
            scopedData?.map((v) => ({
              label: v.value === '' ? BLANK_VALUE : v.value,
              value: v.value,
              inScope: v.inScope,
            })) ?? [],
          )
          .setDefaultOptions([...defaultOptions]);
      } else if (config.type === 'range') {
        const range = data as Int32Range;
        options[filterKey] = new RangeFilterCategoryBuilder()
          .setLabel(column.header as string)
          .setMin(range?.min ?? 0)
          .setMax(range?.max ?? 10000);
      }
    }
    return options;
  }, [filterOptionsQuery.data, hasUrlFiltersParam, isAllTab]);

  useEffect(() => {
    setFilterOptions(nextFilterOptions);
  }, [nextFilterOptions]);

  const onApplyFilter = useCallback(() => {
    onApply?.();
  }, [onApply]);

  return {
    filterValues,
    aip160: combinedFilter,
    onApplyFilter,
    isLoading: filterOptionsQuery.isLoading,
    warnings,
    scopedProductType: filterOptionsQuery.data?.scopedProductType,
    setFiltersBatch,
  };
};
