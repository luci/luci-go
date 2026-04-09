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
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import {
  GetProductCatalogFilterValuesResponse,
  Int32Range,
  ProductCatalogEntry,
  ProductCatalogFilterValue,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { COLUMNS } from './product_catalogue_columns';

const FILTERS: Partial<
  Record<
    keyof ProductCatalogEntry,
    { type: 'string_list' | 'range'; filterKey: string }
  >
> = {
  productCatalogId: { type: 'string_list', filterKey: 'product_catalog_id' },
  productName: { type: 'string_list', filterKey: 'product_name' },
  gpn: { type: 'string_list', filterKey: 'gpn' },
  resourceType: { type: 'string_list', filterKey: 'resource_type' },
  fleetPlmStatus: { type: 'string_list', filterKey: 'fleet_plm_status' },
  r11n: { type: 'string_list', filterKey: 'r11n' },
  numberOfDevicesPerRack: {
    type: 'range',
    filterKey: 'number_of_devices_per_rack',
  },
  productType: { type: 'string_list', filterKey: 'product_type' },
};

export const useProductCatalogFilters = (onApply?: () => void) => {
  const [filterOptions, setFilterOptions] = useState<
    Record<string, StringListFilterCategoryBuilder | RangeFilterCategoryBuilder>
  >({});
  const { filterValues, aip160 } = useFilters(filterOptions, {
    allowExtraKeys: true,
  });

  const client = useFleetConsoleClient();
  const filterOptionsQuery = useQuery({
    ...client.GetProductCatalogFilterValues.query({ filter: aip160 }),
    placeholderData: keepPreviousData,
  });

  const nextFilterOptions = useMemo(() => {
    const options: Record<
      string,
      StringListFilterCategoryBuilder | RangeFilterCategoryBuilder
    > = {};
    for (const column of COLUMNS) {
      const accessorKey = column.accessorKey as keyof ProductCatalogEntry;
      const config = FILTERS[accessorKey];
      if (!config) {
        continue;
      }

      const responseKey =
        accessorKey as keyof GetProductCatalogFilterValuesResponse;
      const data = filterOptionsQuery.data?.[responseKey];
      const filterKey = `"${config.filterKey}"`;
      const scopedKey =
        `scoped${accessorKey.charAt(0).toUpperCase()}${accessorKey.slice(1)}` as keyof GetProductCatalogFilterValuesResponse;
      const scopedData = filterOptionsQuery.data?.[
        scopedKey
      ] as ProductCatalogFilterValue[];

      if (config.type === 'string_list') {
        options[filterKey] = new StringListFilterCategoryBuilder()
          .setLabel(column.header as string)
          .setOptions(
            scopedData?.map((v) => {
              return {
                label: v.value === '' ? BLANK_VALUE : v.value,
                value: `"${v.value}"`,
                inScope: v.inScope,
              };
            }) ?? [],
          );
      } else if (config.type === 'range') {
        const range = data as Int32Range;
        options[filterKey] = new RangeFilterCategoryBuilder()
          .setLabel(column.header as string)
          .setMin(range?.min ?? 0)
          .setMax(range?.max ?? 10000);
      }
    }
    return options;
  }, [filterOptionsQuery.data]);

  useEffect(() => {
    setFilterOptions(nextFilterOptions);
  }, [nextFilterOptions]);

  const onApplyFilter = useCallback(() => {
    onApply?.();
  }, [onApply]);

  return {
    filterValues,
    aip160,
    onApplyFilter,
    isLoading: filterOptionsQuery.isLoading,
  };
};
