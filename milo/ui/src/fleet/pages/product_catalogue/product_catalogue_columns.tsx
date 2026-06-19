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
import { ProductCatalogEntry } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export const COLUMNS: MRT_ColumnDef<ProductCatalogEntry>[] &
  { accessorKey: keyof ProductCatalogEntry }[] = [
  {
    accessorKey: 'productCatalogId',
    header: 'Product Catalog ID',
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
    Cell: renderCellWithLink<ProductCatalogEntry>({
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
];
