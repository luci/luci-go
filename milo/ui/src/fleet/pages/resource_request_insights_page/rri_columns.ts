// Copyright 2025 The LUCI Authors.
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

import { GridColDef } from '@mui/x-data-grid';

import { toIsoString } from '@/fleet/utils/dates';
import {
  ResourceRequest,
  ResourceRequest_Status,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { fulfillmentStatusDisplayValueMap } from './fulfillment_status';

interface ColumnDescriptor {
  id: string;
  gridColDef: GridColDef;
  valueGetter: (rr: ResourceRequest) => string;
}

export const rriColumns = [
  {
    id: 'rr_id',
    gridColDef: {
      field: 'id',
      headerName: 'RR ID',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => rr.rrId,
  },
  {
    id: 'resource_details',
    gridColDef: {
      field: 'resource_details',
      headerName: 'Resource Details',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => rr.resourceDetails,
  },
  {
    id: 'expected_eta',
    gridColDef: {
      field: 'expected_eta',
      headerName: 'Estimated Delivery Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => toIsoString(rr.expectedEta),
  },
  {
    id: 'fulfillment_status',
    gridColDef: {
      field: 'fulfillment_status',
      headerName: 'Fulfillment Status',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) =>
      rr.fulfillmentStatus !== undefined
        ? fulfillmentStatusDisplayValueMap[
            ResourceRequest_Status[
              rr.fulfillmentStatus
            ] as keyof typeof ResourceRequest_Status
          ]
        : '',
  },
  {
    id: 'material_sourcing_actual_delivery_date',
    gridColDef: {
      field: 'material_sourcing_actual_delivery_date',
      headerName: 'Material Sourcing Estimated Delivery Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) =>
      toIsoString(rr.procurementActualDeliveryDate),
  },
  {
    id: 'build_actual_delivery_date',
    gridColDef: {
      field: 'build_actual_delivery_date',
      headerName: 'Build Estimated Delivery Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) =>
      toIsoString(rr.buildActualDeliveryDate),
  },
  {
    id: 'qa_actual_delivery_date',
    gridColDef: {
      field: 'qa_actual_delivery_date',
      headerName: 'QA Estimated Delivery Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => toIsoString(rr.qaActualDeliveryDate),
  },
  {
    id: 'config_actual_delivery_date',
    gridColDef: {
      field: 'config_actual_delivery_date',
      headerName: 'Config Estimated Delivery Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) =>
      toIsoString(rr.configActualDeliveryDate),
  },
] as const satisfies readonly ColumnDescriptor[];

export type ResourceRequestColumnKey = (typeof rriColumns)[number]['id'];
export type ResourceRequestColumnName =
  (typeof rriColumns)[number]['gridColDef']['headerName'];

export const DEFAULT_SORT_COLUMN: ColumnDescriptor =
  rriColumns.find((c) => c.id === 'rr_id') ?? rriColumns[0];

export const getColumnByField = (
  field: string,
): ColumnDescriptor | undefined => {
  return rriColumns.find((c) => c.gridColDef.field === field);
};
