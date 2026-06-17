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

import { Theme } from '@mui/material/styles';
import { MRT_TableOptions } from 'material-react-table';

import { BreakdownTableConfig_BreakdownAggregation } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * Common configuration for Material React Table.
 */
export const COMMON_MRT_CONFIG: Partial<
  MRT_TableOptions<Record<string, number | string | null | undefined>>
> = {
  enableTopToolbar: false,
  enableColumnActions: false,
  enableGlobalFilter: false,
  enableDensityToggle: false,
  enableFullScreenToggle: false,
  enableHiding: false,
  enableFilters: false,
  layoutMode: 'semantic',
  muiTableHeadRowProps: {
    sx: {
      backgroundColor: (theme: Theme) => theme.palette.background.paper,
      borderBottom: '1px solid',
      borderColor: 'divider',
      boxShadow: 'none',
    },
  },
  muiTableHeadCellProps: {
    sx: {
      fontWeight: (theme: Theme) => theme.typography.fontWeightBold,
      fontSize: (theme: Theme) => theme.typography.caption.fontSize,
      borderRight: '1px solid',
      borderColor: 'divider',
    },
  },
  muiTableBodyProps: {
    sx: {
      '& tr:nth-of-type(odd) > td': {
        backgroundColor: 'transparent',
      },
      '& td': {
        borderBottom: '1px solid',
        borderRight: '1px solid',
        borderColor: 'divider',
        py: 1,
      },
    },
  },
  muiTableContainerProps: {
    sx: { overflowX: 'auto' },
  },
  muiTablePaperProps: {
    sx: {
      m: 0,
      borderRadius: 0,
      boxShadow: 'none',
      border: 'none',
    },
    elevation: 0,
  },
};

/**
 * Labels for breakdown aggregations.
 */
export const BREAKDOWN_AGGREGATION_LABELS: Record<
  BreakdownTableConfig_BreakdownAggregation,
  string
> = {
  [BreakdownTableConfig_BreakdownAggregation.BREAKDOWN_AGGREGATION_UNSPECIFIED]:
    'UNKNOWN',
  [BreakdownTableConfig_BreakdownAggregation.COUNT]: 'COUNT',
  [BreakdownTableConfig_BreakdownAggregation.MIN]: 'MIN',
  [BreakdownTableConfig_BreakdownAggregation.MAX]: 'MAX',
  [BreakdownTableConfig_BreakdownAggregation.MEAN]: 'MEAN',
  [BreakdownTableConfig_BreakdownAggregation.P50]: 'P50',
  [BreakdownTableConfig_BreakdownAggregation.P75]: 'P75',
  [BreakdownTableConfig_BreakdownAggregation.P90]: 'P90',
  [BreakdownTableConfig_BreakdownAggregation.P99]: 'P99',
  [BreakdownTableConfig_BreakdownAggregation.DELTA_COUNT]: 'Δ COUNT',
  [BreakdownTableConfig_BreakdownAggregation.DELTA_MIN]: 'Δ MIN',
  [BreakdownTableConfig_BreakdownAggregation.DELTA_MAX]: 'Δ MAX',
  [BreakdownTableConfig_BreakdownAggregation.DELTA_MEAN]: 'Δ MEAN',
  [BreakdownTableConfig_BreakdownAggregation.DELTA_P50]: 'Δ P50',
  [BreakdownTableConfig_BreakdownAggregation.DELTA_P75]: 'Δ P75',
  [BreakdownTableConfig_BreakdownAggregation.DELTA_P90]: 'Δ P90',
  [BreakdownTableConfig_BreakdownAggregation.DELTA_P99]: 'Δ P99',
};

/**
 * List of aggregations displayed in the breakdown selection dropdown.
 */
export const BREAKDOWN_AGGREGATIONS_LIST: readonly BreakdownTableConfig_BreakdownAggregation[] =
  [
    BreakdownTableConfig_BreakdownAggregation.COUNT,
    BreakdownTableConfig_BreakdownAggregation.MIN,
    BreakdownTableConfig_BreakdownAggregation.MAX,
    BreakdownTableConfig_BreakdownAggregation.P50,
    BreakdownTableConfig_BreakdownAggregation.P75,
    BreakdownTableConfig_BreakdownAggregation.P90,
    BreakdownTableConfig_BreakdownAggregation.P99,
    BreakdownTableConfig_BreakdownAggregation.MEAN,
    BreakdownTableConfig_BreakdownAggregation.DELTA_COUNT,
    BreakdownTableConfig_BreakdownAggregation.DELTA_MIN,
    BreakdownTableConfig_BreakdownAggregation.DELTA_MAX,
    BreakdownTableConfig_BreakdownAggregation.DELTA_P50,
    BreakdownTableConfig_BreakdownAggregation.DELTA_P75,
    BreakdownTableConfig_BreakdownAggregation.DELTA_P90,
    BreakdownTableConfig_BreakdownAggregation.DELTA_P99,
    BreakdownTableConfig_BreakdownAggregation.DELTA_MEAN,
  ];

/**
 * List of required aggregations for the breakdown table that cannot be disabled.
 */
export const REQUIRED_BREAKDOWN_AGGREGATIONS: readonly BreakdownTableConfig_BreakdownAggregation[] =
  [
    BreakdownTableConfig_BreakdownAggregation.COUNT,
    BreakdownTableConfig_BreakdownAggregation.MIN,
    BreakdownTableConfig_BreakdownAggregation.MAX,
  ];
