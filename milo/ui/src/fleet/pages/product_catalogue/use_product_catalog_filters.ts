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
import { useCallback, useMemo } from 'react';

import { RangeFilterCategoryBuilder } from '@/fleet/components/filters/range_filter';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { useFilters } from '@/fleet/components/filters/use_filters';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import {
  GetProductCatalogFilterValuesResponse,
  Int32Range,
  ProductCatalogEntry,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

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
  r11nList: { type: 'string_list', filterKey: 'r11n' },
  numberOfDevicesPerRack: {
    type: 'range',
    filterKey: 'number_of_devices_per_rack',
  },
  productType: { type: 'string_list', filterKey: 'product_type' },
};

export const useProductCatalogFilters = (onApply?: () => void) => {
  const client = useFleetConsoleClient();
  const filterOptionsQuery = useQuery({
    ...client.GetProductCatalogFilterValues.query({}),
    placeholderData: keepPreviousData,
  });

  const filterOptions = useMemo(() => {
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

      // TODO: remove this line once we rename r11nList to r11n in the proto
      const responseKey =
        accessorKey === 'r11nList'
          ? 'r11n'
          : (accessorKey as keyof GetProductCatalogFilterValuesResponse);
      const data = filterOptionsQuery.data?.[responseKey];
      const filterKey = `"${config.filterKey}"`;

      if (config.type === 'string_list') {
        const stringList = data as string[];
        options[filterKey] = new StringListFilterCategoryBuilder()
          .setLabel(column.header as string)
          .setOptions(
            stringList?.map((val) => ({
              label: val,
              key: `"${val}"`,
            })) ?? [],
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
  }, [filterOptionsQuery]);

  const { filterValues, aip160 } = useFilters(filterOptions);

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
