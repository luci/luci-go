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

import ErrorIcon from '@mui/icons-material/Error';
import { Box } from '@mui/material';
import { GridColDef, GridRenderCellParams } from '@mui/x-data-grid';
import { DateTime } from 'luxon';

import { toIsoString } from '@/fleet/utils/dates';
import {
  ResourceRequest,
  ResourceRequest_Status,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { fulfillmentStatusDisplayValueMap } from './fulfillment_status';

export interface ColumnDescriptor {
  id: string;
  gridColDef: GridColDef;
  valueGetter: (rr: ResourceRequest) => string;
  isDefault: boolean;
}

// RriGridRow describes the fields within a row in the UI.
//
// This allows rows to reference other rows in the column definitions.
interface RriGridRow {
  id: string;
  resource_details: string;
  expected_eta: string;
  fulfillment_status: string;
  material_sourcing_target_delivery_date: string;
  material_sourcing_actual_delivery_date: string;
  build_actual_delivery_date: string;
  qa_actual_delivery_date: string;
  config_actual_delivery_date: string;
}

// isDateAfter compares two dates objects.
//
// Returns true if date1 is after date2.
function isDateAfter(date1: string, date2: string): boolean {
  if (!date1 || !date2) {
    return false;
  }
  const dt1 = DateTime.fromISO(date1);
  const dt2 = DateTime.fromISO(date2);
  if (!dt1 || !dt2) {
    // Should not happen if DateOnly objects are valid and toLuxonDateTime handles them
    return false;
  }
  return dt1 > dt2;
}

// createDateRenderCell renders the grid cell with a date and other icons.
function createDateRenderCell(
  expectedDateExtractor: (rr: RriGridRow) => string | '',
  actualDateExtractor: (rr: RriGridRow) => string | '',
): (props: GridRenderCellParams) => React.ReactElement {
  const DateRenderCell = (props: GridRenderCellParams) => {
    const rr = props.row;
    const displayValue = props.value as string;

    const expectedDate = expectedDateExtractor(rr);
    const actualDate = actualDateExtractor(rr);
    const late = isDateAfter(actualDate, expectedDate);

    return (
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          width: '100%',
          height: '100%',
        }}
      >
        <span>{displayValue}</span>
        {late && <ErrorIcon style={{ color: 'red' }} />}
      </Box>
    );
  };
  return DateRenderCell;
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
    isDefault: true,
  },
  {
    id: 'resource_details',
    gridColDef: {
      field: 'resource_details',
      headerName: 'Resource Details',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => rr.resourceDetails,
    isDefault: true,
  },
  {
    id: 'expected_eta',
    gridColDef: {
      field: 'expected_eta',
      headerName: 'Estimated Delivery Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => toIsoString(rr.expectedEta),
    isDefault: true,
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
    isDefault: true,
  },
  {
    id: 'material_sourcing_target_delivery_date',
    gridColDef: {
      field: 'material_sourcing_target_delivery_date',
      headerName: 'Material Sourcing Target Delivery Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) =>
      toIsoString(rr.procurementTargetDeliveryDate),
    isDefault: false,
  },
  {
    id: 'material_sourcing_actual_delivery_date',
    gridColDef: {
      field: 'material_sourcing_actual_delivery_date',
      headerName: 'Material Sourcing Estimated Delivery Date',
      flex: 1,
      renderCell: createDateRenderCell(
        (rr: RriGridRow) => rr.material_sourcing_target_delivery_date,
        (rr: RriGridRow) => rr.material_sourcing_actual_delivery_date,
      ),
    },
    valueGetter: (rr: ResourceRequest) =>
      toIsoString(rr.procurementActualDeliveryDate),
    isDefault: true,
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
    isDefault: true,
  },
  {
    id: 'qa_actual_delivery_date',
    gridColDef: {
      field: 'qa_actual_delivery_date',
      headerName: 'QA Estimated Delivery Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => toIsoString(rr.qaActualDeliveryDate),
    isDefault: true,
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
    isDefault: true,
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
