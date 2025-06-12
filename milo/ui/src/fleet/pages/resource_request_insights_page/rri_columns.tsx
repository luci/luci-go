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
import { Duration } from 'luxon';

import { renderCellWithLink } from '@/fleet/components/table/cell_with_link';
import { toIsoString, toLuxonDateTime } from '@/fleet/utils/dates';
import { DateOnly } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';
import {
  ResourceRequest,
  ResourceRequest_Status,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { fulfillmentStatusDisplayValueMap } from './fulfillment_status';

export interface RriColumnDescriptor {
  id: string;
  gridColDef: GridColDef & { field: keyof RriGridRow };
  assignValue: (rr: ResourceRequest, row: RriGridRow) => void;
  isDefault: boolean;
}

interface DateWithOverdueData {
  value: string;
  overdue: Duration;
}

// RriGridRow describes the fields within a row in the UI.
export interface RriGridRow {
  id: string;
  rrId: string;
  resource_request_bug_id: string;
  resource_details: string;
  expected_eta: string;
  fulfillment_status: string;
  material_sourcing_actual_delivery_date: DateWithOverdueData;
  build_actual_delivery_date: DateWithOverdueData;
  qa_actual_delivery_date: DateWithOverdueData;
  config_actual_delivery_date: DateWithOverdueData;
  customer: string;
  resource_name: string;
  accepted_quantity: string;
  criticality: string;
  request_approval: string;
  resource_pm: string;
  fulfillment_channel: string;
  execution_status: string;
  resource_groups: string;
}

const getDateWithOverdueData = (
  resourceRequest: ResourceRequest,
  actualDeliveryDate?: DateOnly,
  targetDeliveryDate?: DateOnly,
): DateWithOverdueData => {
  if (resourceRequest.fulfillmentStatus === ResourceRequest_Status.COMPLETE) {
    return {
      value: toIsoString(actualDeliveryDate),
      overdue: Duration.fromObject({ days: 0 }),
    };
  }

  const overdue =
    actualDeliveryDate && targetDeliveryDate
      ? toLuxonDateTime(actualDeliveryDate)!.diff(
          toLuxonDateTime(targetDeliveryDate)!,
          'days',
        )
      : Duration.fromObject({ days: 0 });

  return {
    value: toIsoString(actualDeliveryDate),
    overdue: overdue,
  };
};

const renderDateCellWithOverdueIndicator = (date: DateWithOverdueData) => {
  return (
    <>
      <span>{date.value}</span>
      {date.overdue.days > 0 && (
        <span css={{ color: 'red', marginLeft: 20 }}>
          {'('}
          {date.overdue
            .shiftTo('years', 'months', 'weeks', 'days')
            .rescale()
            .toHuman({ unitDisplay: 'narrow' })}
          {')'}
        </span>
      )}
    </>
  );
};

export const RRI_COLUMNS = [
  {
    id: 'rr_id',
    gridColDef: {
      field: 'rrId',
      headerName: 'RR ID',
      flex: 1,
    },
    assignValue: (rr, row) => (row.rrId = rr.rrId),
    isDefault: true,
  },
  {
    id: 'resource_request_bug_id',
    gridColDef: {
      field: 'resource_request_bug_id',
      headerName: 'Resource Request Bug ID',
      flex: 1,
      renderCell: renderCellWithLink((v) => `http://${v}`),
    },
    assignValue: (rr, row) =>
      (row.resource_request_bug_id = rr.resourceRequestBugId ?? ''),
    isDefault: true,
  },
  {
    id: 'resource_details',
    gridColDef: {
      field: 'resource_details',
      headerName: 'Resource Details',
      flex: 1,
    },
    assignValue: (rr, row) => (row.resource_details = rr.resourceDetails),
    isDefault: true,
  },
  {
    id: 'expected_eta',
    gridColDef: {
      field: 'expected_eta',
      headerName: 'Estimated Delivery Date',
      flex: 1,
    },
    assignValue: (rr, row) => (row.expected_eta = toIsoString(rr.expectedEta)),
    isDefault: true,
  },
  {
    id: 'fulfillment_status',
    gridColDef: {
      field: 'fulfillment_status',
      headerName: 'Fulfillment Status',
      flex: 1,
    },
    assignValue: (rr, row) =>
      (row.fulfillment_status =
        rr.fulfillmentStatus !== undefined
          ? fulfillmentStatusDisplayValueMap[
              ResourceRequest_Status[
                rr.fulfillmentStatus
              ] as keyof typeof ResourceRequest_Status
            ]
          : ''),
    isDefault: true,
  },
  {
    id: 'material_sourcing_actual_delivery_date',
    gridColDef: {
      field: 'material_sourcing_actual_delivery_date',
      headerName: 'Material Sourcing Estimated Delivery Date',
      flex: 1,
      renderCell: (params) =>
        renderDateCellWithOverdueIndicator(
          (params.row as RriGridRow).material_sourcing_actual_delivery_date,
        ),
    },
    assignValue: (rr, row) =>
      (row.material_sourcing_actual_delivery_date = getDateWithOverdueData(
        rr,
        rr.procurementActualDeliveryDate,
        rr.procurementTargetDeliveryDate,
      )),
    isDefault: true,
  },
  {
    id: 'build_actual_delivery_date',
    gridColDef: {
      field: 'build_actual_delivery_date',
      headerName: 'Build Estimated Delivery Date',
      flex: 1,
      renderCell: (params) =>
        renderDateCellWithOverdueIndicator(
          (params.row as RriGridRow).build_actual_delivery_date,
        ),
    },
    assignValue: (rr, row) => {
      row.build_actual_delivery_date = getDateWithOverdueData(
        rr,
        rr.buildActualDeliveryDate,
        rr.buildTargetDeliveryDate,
      );
    },
    isDefault: true,
  },
  {
    id: 'qa_actual_delivery_date',
    gridColDef: {
      field: 'qa_actual_delivery_date',
      headerName: 'QA Estimated Delivery Date',
      flex: 1,
      renderCell: (params) =>
        renderDateCellWithOverdueIndicator(
          (params.row as RriGridRow).qa_actual_delivery_date,
        ),
    },
    assignValue: (rr, row) =>
      (row.qa_actual_delivery_date = getDateWithOverdueData(
        rr,
        rr.qaActualDeliveryDate,
        rr.qaTargetDeliveryDate,
      )),
    isDefault: true,
  },
  {
    id: 'config_actual_delivery_date',
    gridColDef: {
      field: 'config_actual_delivery_date',
      headerName: 'Config Estimated Delivery Date',
      flex: 1,
      renderCell: (params) =>
        renderDateCellWithOverdueIndicator(
          (params.row as RriGridRow).config_actual_delivery_date,
        ),
    },
    assignValue: (rr, row) =>
      (row.config_actual_delivery_date = getDateWithOverdueData(
        rr,
        rr.configActualDeliveryDate,
        rr.configTargetDeliveryDate,
      )),
    isDefault: true,
  },
  {
    id: 'customer',
    gridColDef: {
      field: 'customer',
      headerName: 'Customer',
      flex: 1,
    },
    assignValue: (rr, row) => (row.customer = rr.customer ?? ''),
    isDefault: true,
  },
  {
    id: 'resource_name',
    gridColDef: {
      field: 'resource_name',
      headerName: 'Resource Name',
      flex: 1,
    },
    assignValue: (rr, row) => (row.resource_name = rr.resourceName ?? ''),
    isDefault: true,
  },
  {
    id: 'accepted_quantity',
    gridColDef: {
      field: 'accepted_quantity',
      headerName: 'Accepted Quantity',
      flex: 1,
      type: 'number',
    },
    assignValue: (rr, row) =>
      (row.accepted_quantity = rr.acceptedQuantity?.toString() ?? ''),
    isDefault: true,
  },
  {
    id: 'criticality',
    gridColDef: {
      field: 'criticality',
      headerName: 'Criticality',
      flex: 1,
    },
    assignValue: (rr, row) => (row.criticality = rr.criticality ?? ''),
    isDefault: true,
  },
  {
    id: 'request_approval',
    gridColDef: {
      field: 'request_approval',
      headerName: 'Request Approval',
      flex: 1,
    },
    assignValue: (rr, row) => (row.request_approval = rr.requestApproval ?? ''),
    isDefault: true,
  },
  {
    id: 'resource_pm',
    gridColDef: {
      field: 'resource_pm',
      headerName: 'Resource PM',
      flex: 1,
    },
    assignValue: (rr, row) => (row.resource_pm = rr.resourcePm ?? ''),
    isDefault: true,
  },
  {
    id: 'fulfillment_channel',
    gridColDef: {
      field: 'fulfillment_channel',
      headerName: 'Fulfillment Channel',
      flex: 1,
    },
    assignValue: (rr, row) =>
      (row.fulfillment_channel = rr.fulfillmentChannel ?? ''),
    isDefault: true,
  },
  {
    id: 'execution_status',
    gridColDef: {
      field: 'execution_status',
      headerName: 'Execution Status',
      flex: 1,
    },
    assignValue: (rr, row) => (row.execution_status = rr.executionStatus ?? ''),
    isDefault: true,
  },
  {
    id: 'resource_groups',
    gridColDef: {
      field: 'resource_groups',
      headerName: 'Resource Groups',
      flex: 1,
    },
    assignValue: (rr, row) =>
      (row.resource_groups = rr.resourceGroups.join(', ')),
    isDefault: true,
  },
] as const satisfies readonly RriColumnDescriptor[];

export type ResourceRequestColumnKey = (typeof RRI_COLUMNS)[number]['id'];

export const DEFAULT_SORT_COLUMN: RriColumnDescriptor =
  RRI_COLUMNS.find((c) => c.id === 'rr_id') ?? RRI_COLUMNS[0];

export const getColumnByField = (
  field: string,
): RriColumnDescriptor | undefined => {
  return RRI_COLUMNS.find((c) => c.gridColDef.field === field);
};
