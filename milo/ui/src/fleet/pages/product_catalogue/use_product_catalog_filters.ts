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
import { useMemo, useCallback } from 'react';

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
  GetGceProductCatalogFilterValuesRequest,
  Int32Range,
  ProductCatalogFilterValue,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { COLUMNS, getColumnsForTab } from './product_catalogue_columns';
import { ProductCatalogTab } from './use_product_catalog_tabs';

export const FILTERS = {
  productCatalogId: { type: 'string_list', filterKey: 'product_catalog_id' },
  productName: { type: 'string_list', filterKey: 'product_name' },
  gpn: { type: 'string_list', filterKey: 'gpn' },
  descriptiveName: { type: 'string_list', filterKey: 'descriptive_name' },
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
  cpuType: { type: 'string_list', filterKey: 'cpu_type' },
  cpuNumPerVm: { type: 'string_list', filterKey: 'cpu_num_per_vm' },
  memoryGbPerVm: { type: 'string_list', filterKey: 'memory_gb_per_vm' },
} satisfies Record<
  string,
  {
    type: 'string_list' | 'range';
    filterKey: string;
  }
>;

export const DEFAULT_FILTER_VALUES: Partial<
  Record<keyof typeof FILTERS, readonly string[]>
> = {
  fleetPlmStatus: ['GA', 'LA', 'NPI'],
};

// Filter keys that only exist in the non-virtual product catalog
export const NONVIRTUAL_ONLY_KEYS = [
  'resource_type',
  'gpn',
  'r11n',
  'number_of_devices_per_rack',
];

// Filter keys that only exist in the GCE product catalog
export const GCE_ONLY_KEYS = ['cpu_type', 'cpu_num_per_vm', 'memory_gb_per_vm'];

/**
 * Merges two lists of scoped filter values by unique value, combining their inScope flags.
 */
function mergeFilterValues(
  nonVirtualValues: readonly ProductCatalogFilterValue[] = [],
  gceValues: readonly ProductCatalogFilterValue[] = [],
): ProductCatalogFilterValue[] {
  const map = new Map<string, boolean>();
  for (const item of nonVirtualValues) {
    if (item.value !== undefined && item.value !== null) {
      map.set(item.value, item.inScope);
    }
  }
  for (const item of gceValues) {
    if (item.value !== undefined && item.value !== null) {
      const existing = map.get(item.value) ?? false;
      map.set(item.value, existing || item.inScope);
    }
  }
  return Array.from(map.entries()).map(([value, inScope]) => ({
    value,
    inScope,
  }));
}

export interface QueryTargets {
  enableNonVirtual: boolean;
  enableGce: boolean;
}

/**
 * Checks if a specific field key is targeted in an AIP-160 filter string,
 * matching word boundaries / quotes / operators to prevent false positives
 * from substrings in values.
 */
export function hasFilterField(
  filters: string | null | undefined,
  fieldKey: string,
): boolean {
  if (!filters) return false;
  const regex = new RegExp(
    `(?:^|[^\\w])"?${fieldKey}"?\\s*(=|!=|:|<=|>=|<|>)`,
    'i',
  );
  return regex.test(filters);
}

/**
 * Synchronously determines whether Non-virtual and/or GCE queries should be enabled
 * based on the active tab and AIP-160 filter string.
 */
export function resolveQueryTargets(
  selectedTab: ProductCatalogTab,
  filtersParam: string | null | undefined,
): QueryTargets {
  if (selectedTab === ProductCatalogTab.GCE) {
    return { enableNonVirtual: false, enableGce: true };
  }
  if (selectedTab !== ProductCatalogTab.ALL) {
    return { enableNonVirtual: true, enableGce: false };
  }

  // On the "All" tab:
  if (!filtersParam) {
    return { enableNonVirtual: true, enableGce: true };
  }

  const hasNonVirtualOnly = NONVIRTUAL_ONLY_KEYS.some((k) =>
    hasFilterField(filtersParam, k),
  );
  const hasGceOnly = GCE_ONLY_KEYS.some((k) => hasFilterField(filtersParam, k));

  if (hasNonVirtualOnly && hasGceOnly) {
    return { enableNonVirtual: false, enableGce: false };
  }
  if (hasNonVirtualOnly) {
    return { enableNonVirtual: true, enableGce: false };
  }
  if (hasGceOnly) {
    return { enableNonVirtual: false, enableGce: true };
  }

  // Inspect product_type if present in filters
  const ptMatches = Array.from(
    filtersParam.matchAll(
      /(?:^|[^\w])"?product_type"?\s*(!=|=)\s*(?:\(([^)]*)\)|("[^"]*"|\S+))/gi,
    ),
  );
  if (ptMatches.length > 0) {
    const isExcluded = ptMatches.some((m) => m[1] === '!=');
    const values = ptMatches
      .flatMap((m) => (m[2] || m[3] || '').split(/,|\s+OR\s+/i))
      .map((v) => v.replace(/["']/g, '').trim().toLowerCase())
      .filter(Boolean);

    if (values.length === 0) {
      return { enableNonVirtual: true, enableGce: true };
    }

    const mentionsGce = values.includes(ProductCatalogTab.GCE);

    if (isExcluded) {
      if (mentionsGce) {
        // GCE is excluded (e.g. product_type != ("gce")), so disable GCE
        return { enableNonVirtual: true, enableGce: false };
      }
      // If a non-virtual type was excluded (e.g. product_type != ("hardware")),
      // both GCE and other non-virtual types remain enabled
      return { enableNonVirtual: true, enableGce: true };
    } else {
      // Inclusion
      const uniqueValues = new Set(values);
      if (mentionsGce && uniqueValues.size === 1) {
        // Only GCE selected
        return { enableNonVirtual: false, enableGce: true };
      }
      if (!mentionsGce) {
        // Only non-virtual types selected, no GCE
        return { enableNonVirtual: true, enableGce: false };
      }
      // Both GCE and non-virtual types selected
      return { enableNonVirtual: true, enableGce: true };
    }
  }

  return { enableNonVirtual: true, enableGce: true };
}

export const useProductCatalogFilters = (
  selectedTab: ProductCatalogTab,
  onApply?: () => void,
) => {
  const [searchParams] = useSyncedSearchParams();
  const filtersParam = searchParams.get(FILTERS_PARAM_KEY);
  const hasUrlFiltersParam = searchParams.has(FILTERS_PARAM_KEY);
  const { trackEvent } = useGoogleAnalytics();

  const onFilterChange = useCallback(
    (searchParams: URLSearchParams) => {
      trackEvent('product_catalogue_search', {
        componentName: 'product_catalogue_filter',
      });
      return searchParams;
    },
    [trackEvent],
  );

  const isAllTab = selectedTab === ProductCatalogTab.ALL;
  const isGceTab = selectedTab === ProductCatalogTab.GCE;

  // Non-virtual tab predicate (e.g. product_type = "hardware")
  const currentTab =
    isAllTab || isGceTab
      ? ''
      : `("${FILTERS.productType.filterKey}" = "${selectedTab}")`;

  const nonVirtualCombinedFilter = combineAipFilters(
    filtersParam || '',
    currentTab,
  );

  const { enableNonVirtual, enableGce } = resolveQueryTargets(
    selectedTab,
    filtersParam,
  );

  const client = useFleetConsoleClient();

  const nonVirtualFilterOptionsQuery = useQuery({
    ...client.GetProductCatalogFilterValues.query({
      filter: nonVirtualCombinedFilter,
    }),
    enabled: enableNonVirtual,
    placeholderData: keepPreviousData,
  });

  const gceFilterOptionsQuery = useQuery({
    ...client.GetGceProductCatalogFilterValues.query(
      GetGceProductCatalogFilterValuesRequest.fromPartial({
        filter: filtersParam || '',
      }),
    ),
    enabled: enableGce,
    placeholderData: keepPreviousData,
  });

  const nonVirtualData = nonVirtualFilterOptionsQuery.data as
    | Record<string, unknown>
    | undefined;
  const gceData = gceFilterOptionsQuery.data as
    | Record<string, unknown>
    | undefined;

  const tabColumns = getColumnsForTab(selectedTab);

  const nextFilterOptions = useMemo(() => {
    const hasData =
      (!isGceTab && nonVirtualFilterOptionsQuery.data) ||
      (isGceTab && gceFilterOptionsQuery.data) ||
      (isAllTab &&
        (nonVirtualFilterOptionsQuery.data || gceFilterOptionsQuery.data));

    if (!hasData) return undefined;

    const options: Record<
      string,
      StringListFilterCategoryBuilder | RangeFilterCategoryBuilder
    > = {};

    for (const column of COLUMNS) {
      if (!('accessorKey' in column) || !column.accessorKey) continue;
      if (!(column.accessorKey in FILTERS)) continue;

      const accessorKey = column.accessorKey as keyof typeof FILTERS;
      const config = FILTERS[accessorKey];

      // Omit columns not visible on the current tab (e.g. cpu_type on hardware tab)
      const isColumnInTab = tabColumns.some(
        (c) => 'accessorKey' in c && c.accessorKey === accessorKey,
      );
      if (!isAllTab && !isColumnInTab) continue;

      // Omit productType filter when not on All tab
      if (accessorKey === 'productType' && !isAllTab) continue;

      const filterKey = `"${config.filterKey}"`;
      const scopedKey = `scoped${accessorKey.charAt(0).toUpperCase()}${accessorKey.slice(1)}`;

      if (config.type === 'string_list') {
        const nonVirtualScoped =
          (nonVirtualData?.[scopedKey] as ProductCatalogFilterValue[]) ?? [];
        const gceScoped =
          (gceData?.[scopedKey] as ProductCatalogFilterValue[]) ?? [];

        let scopedData: readonly ProductCatalogFilterValue[] = [];
        if (NONVIRTUAL_ONLY_KEYS.includes(config.filterKey)) {
          scopedData = nonVirtualScoped;
        } else if (GCE_ONLY_KEYS.includes(config.filterKey)) {
          scopedData = gceScoped;
        } else if (isGceTab) {
          scopedData = gceScoped;
        } else if (isAllTab) {
          scopedData = mergeFilterValues(nonVirtualScoped, gceScoped);
          if (
            accessorKey === 'productType' &&
            !scopedData.some((v) => v.value === 'gce')
          ) {
            scopedData = [...scopedData, { value: 'gce', inScope: true }];
          }
        } else {
          scopedData = nonVirtualScoped;
        }

        const defaultOptions = hasUrlFiltersParam
          ? []
          : (DEFAULT_FILTER_VALUES[accessorKey] ?? []);

        options[filterKey] = new StringListFilterCategoryBuilder()
          .setLabel(column.header as string)
          .setOptions(
            scopedData.map((v) => ({
              label: v.value === '' ? BLANK_VALUE : v.value,
              value: v.value,
              inScope: v.inScope,
            })),
          )
          .setDefaultOptions([...defaultOptions]);
      } else if (config.type === 'range') {
        const range = nonVirtualData?.[
          accessorKey as keyof typeof nonVirtualData
        ] as Int32Range | undefined;
        options[filterKey] = new RangeFilterCategoryBuilder()
          .setLabel(column.header as string)
          .setMin(range?.min ?? 0)
          .setMax(range?.max ?? 10000);
      }
    }
    return options;
  }, [
    isAllTab,
    isGceTab,
    nonVirtualFilterOptionsQuery.data,
    gceFilterOptionsQuery.data,
    tabColumns,
    nonVirtualData,
    gceData,
    hasUrlFiltersParam,
  ]);

  const isFiltersLoading =
    (enableNonVirtual && nonVirtualFilterOptionsQuery.isLoading) ||
    (enableGce && gceFilterOptionsQuery.isLoading);

  const { filterValues, aip160, warnings } = useFilters(nextFilterOptions, {
    areFilterValuesLoading: isFiltersLoading,
    onFilterChange,
  });

  const onApplyFilter = useCallback(() => {
    onApply?.();
  }, [onApply]);

  const nonVirtualFilter = isAllTab
    ? aip160()
    : combineAipFilters(aip160(), currentTab);
  const gceFilter = aip160();

  return {
    filterValues,
    nonVirtualFilter,
    gceFilter,
    isNonVirtualQueryEnabled: enableNonVirtual,
    isGceQueryEnabled: enableGce,
    onApplyFilter,
    isLoading: isFiltersLoading,
    warnings,
  };
};
