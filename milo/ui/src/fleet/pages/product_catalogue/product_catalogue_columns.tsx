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

import { MRT_ColumnDef } from 'material-react-table';

import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { renderCellWithLink } from '@/fleet/components/table/cell_with_link';
import { generateCatalogDetailsURL } from '@/fleet/constants/paths';

import { UnifiedProductCatalogEntry, CatalogColumnKey } from './types';
import { ProductCatalogTab } from './use_product_catalog_tabs';

export const COLUMNS: MRT_ColumnDef<UnifiedProductCatalogEntry>[] &
  { accessorKey: keyof UnifiedProductCatalogEntry }[] = [
  {
    accessorKey: 'productCatalogId',
    header: 'Product Catalog ID',
    Cell: renderCellWithLink<UnifiedProductCatalogEntry>({
      linkGenerator: (value) => generateCatalogDetailsURL(value ?? ''),
      newTab: false,
    }),
  },
  {
    accessorKey: 'productName',
    header: 'Product Name',
  },
  {
    accessorKey: 'gpn',
    header: 'GPN',
  },
  {
    accessorKey: 'descriptiveName',
    header: 'Descriptive Name',
  },
  {
    accessorKey: 'resourceType',
    header: 'Resource Type',
  },
  {
    accessorKey: 'fleetPlmStatus',
    header: 'Fleet PLM Status',
  },
  {
    accessorKey: 'r11n',
    header: 'R11N',
    Cell: renderCellWithLink<UnifiedProductCatalogEntry>({
      linkGenerator: (value) => {
        const trimmed = (value ?? '').trim();
        if (!trimmed) {
          return '';
        }
        return `http://go/ngp-npi/r11n/${trimmed.toLowerCase()}`;
      },
    }),
    sortingFn: (rowA, rowB) => {
      const valA = labelValuesToString(rowA.original.r11n ?? []);
      const valB = labelValuesToString(rowB.original.r11n ?? []);
      return valA.localeCompare(valB);
    },
  },
  {
    accessorKey: 'numberOfDevicesPerRack',
    header: 'Number of Devices Per Rack',
  },
  {
    accessorKey: 'unitCost',
    header: 'Unit Cost',
  },
  {
    accessorKey: 'productType',
    header: 'Product Type',
  },
  {
    accessorKey: 'cpuType',
    header: 'CPU Type',
  },
  {
    accessorKey: 'cpuNumPerVm',
    header: 'CPU Count Per VM',
  },
  {
    accessorKey: 'memoryGbPerVm',
    header: 'Memory (GB) Per VM',
  },
];

export const ALL_TAB_COLUMN_KEYS: readonly CatalogColumnKey[] = [
  'productCatalogId',
  'productName',
  'gpn',
  'descriptiveName',
  'productType',
  'fleetPlmStatus',
  'resourceType',
  'r11n',
  'numberOfDevicesPerRack',
  'cpuType',
  'cpuNumPerVm',
  'memoryGbPerVm',
  'unitCost',
];

export const GCE_COLUMN_KEYS: readonly CatalogColumnKey[] = [
  'productCatalogId',
  'productName',
  'descriptiveName',
  'cpuType',
  'cpuNumPerVm',
  'memoryGbPerVm',
  'fleetPlmStatus',
];

export const ANDROID_TESTBED_COLUMN_KEYS: readonly CatalogColumnKey[] = [
  'productCatalogId',
  'productName',
  'gpn',
  'descriptiveName',
  'resourceType',
  'fleetPlmStatus',
  'r11n',
  'numberOfDevicesPerRack',
  'unitCost',
];

export const HARDWARE_COLUMN_KEYS: readonly CatalogColumnKey[] = [
  'productCatalogId',
  'productName',
  'gpn',
  'descriptiveName',
  'resourceType',
  'fleetPlmStatus',
  'r11n',
  'numberOfDevicesPerRack',
  'unitCost',
];

export const OS_TESTBED_COLUMN_KEYS: readonly CatalogColumnKey[] = [
  'productCatalogId',
  'productName',
  'gpn',
  'descriptiveName',
  'resourceType',
  'fleetPlmStatus',
  'r11n',
  'numberOfDevicesPerRack',
  'unitCost',
];

export const PERIPHERALS_COLUMN_KEYS: readonly CatalogColumnKey[] = [
  'productCatalogId',
  'productName',
  'gpn',
  'descriptiveName',
  'resourceType',
  'fleetPlmStatus',
  'r11n',
  'numberOfDevicesPerRack',
  'unitCost',
];

export const TAB_COLUMN_KEYS: Record<
  ProductCatalogTab | string,
  readonly CatalogColumnKey[]
> = {
  [ProductCatalogTab.ALL]: ALL_TAB_COLUMN_KEYS,
  [ProductCatalogTab.GCE]: GCE_COLUMN_KEYS,
  [ProductCatalogTab.ANDROID_TESTBED]: ANDROID_TESTBED_COLUMN_KEYS,
  [ProductCatalogTab.HARDWARE]: HARDWARE_COLUMN_KEYS,
  [ProductCatalogTab.OS_TESTBED]: OS_TESTBED_COLUMN_KEYS,
  [ProductCatalogTab.PERIPHERALS]: PERIPHERALS_COLUMN_KEYS,
};

export const getColumnsForTab = (tab: ProductCatalogTab | string) => {
  const keys = TAB_COLUMN_KEYS[tab] || ALL_TAB_COLUMN_KEYS;
  return COLUMNS.filter(
    (column) =>
      column.accessorKey !== undefined &&
      keys.includes(column.accessorKey as keyof UnifiedProductCatalogEntry),
  ) as typeof COLUMNS;
};
