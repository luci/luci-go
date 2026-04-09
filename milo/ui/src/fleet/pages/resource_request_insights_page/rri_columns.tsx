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
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { Box, IconButton } from '@mui/material';

import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { FilterOption } from '@/fleet/components/fc_data_table/mrt_filter_menu_item';
import { FC_ColumnDef } from '@/fleet/components/fc_data_table/use_fc_data_table';
import {
  GetResourceRequestsMultiselectFilterValuesResponse,
  ResourceRequest_Status,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { fulfillmentStatusDisplayValueMap } from './fulfillment_status';
import { formatBugUrl } from './rri_url_utils';
import { RriGridRow, DateWithOverdueData } from './rri_utils';

const getOverdueColor = (overdueDays: number) => {
  return overdueDays < 30 ? 'warning.main' : 'error.main';
};

// eslint-disable-next-line react-refresh/only-export-components
export const STATUS_FILTER_OPTIONS: FilterOption[] = Object.keys(
  fulfillmentStatusDisplayValueMap,
).map((k) => ({
  value: k,
  label:
    fulfillmentStatusDisplayValueMap[k as keyof typeof ResourceRequest_Status],
}));

const renderDateCellWithOverdueIndicator = (date: DateWithOverdueData) => {
  const overdueDays = date.overdue.shiftTo('days').days;

  return (
    <EllipsisTooltip>
      <span>{date.value}</span>
      {overdueDays > 0 && (
        <Box
          component="span"
          sx={{ color: getOverdueColor(overdueDays), ml: 2.5 }}
        >
          {'('}
          {date.overdue.shiftTo('days').toHuman({ unitDisplay: 'narrow' })}
          {')'}
        </Box>
      )}
    </EllipsisTooltip>
  );
};

const renderSlippageCell = (slippage: number) => {
  return (
    <Box
      component="span"
      sx={slippage > 0 ? { color: getOverdueColor(slippage) } : undefined}
    >
      {slippage}d
    </Box>
  );
};

export type RriColumnDef<Key extends keyof RriGridRow> = FC_ColumnDef<
  RriGridRow,
  RriGridRow[Key]
> & {
  accessorKey: Key;
  rriFilterKey?: keyof GetResourceRequestsMultiselectFilterValuesResponse;
};

export const COLUMNS = {
  resource_details: {
    accessorKey: 'resource_details',
    header: 'Resource Details',
    size: 180,
    rriFilterKey: 'resourceDetails',
    Cell: (x) => {
      return (
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            minWidth: 0,
            width: '100%',
            overflow: 'hidden',
          }}
        >
          <sup style={{ display: 'inline-flex', flexShrink: 0 }}>
            <IconButton
              href={formatBugUrl(x.row.original.resource_request_bug_id)}
              target="_blank"
              color="primary"
              sx={{ padding: 0, marginRight: '4px' }}
            >
              <OpenInNewIcon sx={{ width: 14, height: 14 }} />
            </IconButton>
          </sup>
          <Box sx={{ minWidth: 0, flex: 1, width: '100%', overflow: 'hidden' }}>
            <EllipsisTooltip>{x.cell.getValue()}</EllipsisTooltip>
          </Box>
        </Box>
      );
    },
  },
  rr_id: {
    accessorKey: 'rr_id',
    header: 'RR ID',
    size: 70,
    rriFilterKey: 'rrIds',
  },
  resource_request_bug_id: {
    accessorKey: 'resource_request_bug_id',
    header: 'RR Bug ID',
    size: 100,
    Cell: (x) => {
      const v = x.cell.getValue();
      if (!v) return null;
      return (
        <a href={formatBugUrl(v)} target="_blank" rel="noopener noreferrer">
          {v}
        </a>
      );
    },
  },
  resource_request_target_delivery_date: {
    accessorKey: 'resource_request_target_delivery_date',
    header: 'Target Date',
    size: 85,
    filterVariant: 'date-range',
  },
  resource_request_actual_delivery_date: {
    accessorKey: 'resource_request_actual_delivery_date',
    header: 'Est. Delivery Date',
    size: 130,
    filterVariant: 'date-range',
    Cell: (x) => renderDateCellWithOverdueIndicator(x.cell.getValue()),
  },
  slippage: {
    accessorKey: 'slippage',
    header: 'Slippage',
    size: 75,
    filterVariant: 'range',
    filterRangeMax: 365,
    muiTableBodyCellProps: {
      align: 'left',
    },
    muiTableHeadCellProps: {
      align: 'left',
    },
    Cell: (x) => renderSlippageCell(x.cell.getValue()),
  },
  fulfillment_status: {
    accessorKey: 'fulfillment_status',
    header: 'Fulfillment Status',
    size: 120,
    filterSelectOptions: STATUS_FILTER_OPTIONS,
  },
  resource_request_status: {
    accessorKey: 'resource_request_status',
    header: 'RR Status',
    size: 110,
    filterSelectOptions: STATUS_FILTER_OPTIONS,
  },
  rr_bug_status: {
    accessorKey: 'rr_bug_status',
    header: 'RR Bug Status',
    size: 120,
    rriFilterKey: 'resourceRequestBugStatus',
  },
  material_sourcing_actual_delivery_date: {
    accessorKey: 'material_sourcing_actual_delivery_date',
    header: 'Material Sourcing Est. Date',
    size: 130,
    filterVariant: 'date-range',
    Cell: (x) => renderDateCellWithOverdueIndicator(x.cell.getValue()),
  },
  build_actual_delivery_date: {
    accessorKey: 'build_actual_delivery_date',
    header: 'Build Est. Date',
    size: 110,
    filterVariant: 'date-range',
    Cell: (x) => renderDateCellWithOverdueIndicator(x.cell.getValue()),
  },
  qa_actual_delivery_date: {
    accessorKey: 'qa_actual_delivery_date',
    header: 'QA Est. Date',
    size: 110,
    filterVariant: 'date-range',
    Cell: (x) => renderDateCellWithOverdueIndicator(x.cell.getValue()),
  },
  config_actual_delivery_date: {
    accessorKey: 'config_actual_delivery_date',
    header: 'Config Est. Date',
    size: 110,
    filterVariant: 'date-range',
    Cell: (x) => renderDateCellWithOverdueIndicator(x.cell.getValue()),
  },
  customer: {
    accessorKey: 'customer',
    header: 'Customer',
    size: 85,
    rriFilterKey: 'customer',
  },
  resource_name: {
    accessorKey: 'resource_name',
    header: 'Resource Name',
    size: 95,
    rriFilterKey: 'resourceName',
  },
  accepted_quantity: {
    accessorKey: 'accepted_quantity',
    header: 'Accepted Quantity',
    size: 70,
    filterVariant: 'range',
  },
  criticality: {
    accessorKey: 'criticality',
    header: 'Criticality',
    size: 80,
    rriFilterKey: 'criticality',
  },
  request_approval: {
    accessorKey: 'request_approval',
    header: 'Request Approval',
    size: 90,
    rriFilterKey: 'requestApproval',
  },
  resource_pm: {
    accessorKey: 'resource_pm',
    header: 'Resource PM',
    size: 85,
    rriFilterKey: 'resourcePm',
  },
  fulfillment_channel: {
    accessorKey: 'fulfillment_channel',
    header: 'Fulfillment Channel',
    size: 100,
    rriFilterKey: 'fulfillmentChannel',
  },
  execution_status: {
    accessorKey: 'execution_status',
    header: 'Execution Status',
    size: 90,
    rriFilterKey: 'executionStatus',
  },
  resource_groups: {
    accessorKey: 'resource_groups',
    header: 'Resource Groups',
    size: 120,
    rriFilterKey: 'resourceGroups',
  },
} satisfies Partial<{
  [Key in keyof RriGridRow]: RriColumnDef<Key>;
}>;

export type ResourceRequestColumnKey = keyof typeof COLUMNS;

export const DEFAULT_SORT_COLUMN_ID = 'slippage';

// eslint-disable-next-line react-refresh/only-export-components
export const DEFAULT_COLUMNS = [
  'resource_details',
  'rr_id',
  'resource_request_target_delivery_date',
  'resource_request_actual_delivery_date',
  'slippage',
  'customer',
  'resource_name',
  'accepted_quantity',
  'request_approval',
  'resource_pm',
  'execution_status',
  'resource_groups',
];
