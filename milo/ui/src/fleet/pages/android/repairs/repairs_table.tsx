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
import {
  MaterialReactTable,
  MRT_ColumnDef,
  MRT_TableOptions,
} from 'material-react-table';
import { useMemo } from 'react';

import { usePagerContext } from '@/common/components/params_pager';
import {
  MrtColumnManager,
  getColumnId,
} from '@/fleet/components/columns/use_mrt_column_management';
import { FleetBottomToolbar } from '@/fleet/components/fc_data_table/fleet_bottom_toolbar';
import { FleetTopToolbar } from '@/fleet/components/fc_data_table/fleet_top_toolbar';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useMrtSortingState } from '@/fleet/hooks/use_mrt_sorting_state';
import { usePager } from '@/fleet/hooks/use_pager';
import { getErrorMessage } from '@/fleet/utils/errors';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getRow, Row } from './repairs_columns.utils';
import { useRepairsFilters } from './use_repairs_filters';

interface RepairsTableProps {
  mrtColumnManager: MrtColumnManager<MRT_ColumnDef<Row>>;
}

export const RepairsTable = ({ mrtColumnManager }: RepairsTableProps) => {
  const pagerCtx = usePagerContext({
    pageSizeOptions: [10, 25, 50, 100],
    defaultPageSize: 100,
  });

  const { pageSize, pageToken } = usePager(pagerCtx);

  const [sorting, onSortingChange, orderByParam] = useMrtSortingState(
    mrtColumnManager.columns.map((c) => ({
      id: getColumnId(c),
    })),
    pagerCtx,
  );

  const {
    filterValues,
    aip160,
    warnings: filterWarnings,
  } = useRepairsFilters(() => {});

  const client = useFleetConsoleClient();

  const repairMetricsList = useQuery({
    ...client.ListRepairMetrics.query({
      platform: Platform.ANDROID,
      filter: filterWarnings.length > 0 ? '' : aip160(),
      pageSize,
      pageToken,
      orderBy: orderByParam,
    }),
    placeholderData: keepPreviousData,
  });

  const data = useMemo(
    () => repairMetricsList.data?.repairMetrics.map(getRow) ?? [],
    [repairMetricsList.data],
  );

  const tableOptions: MRT_TableOptions<Row> = useMemo(
    () => ({
      columns: mrtColumnManager.columns,
      data,
      enableColumnActions: true,
      enableColumnFilters: false,
      manualFiltering: true,
      filterValues,
      error: repairMetricsList.error
        ? getErrorMessage(repairMetricsList.error, 'get repair metrics')
        : undefined,
      enableRowSelection: true,
      getRowId: (row) => row.lab_name + row.host_group + row.run_target,
      onSortingChange,
      enablePagination: false,
      renderTopToolbarCustomActions: ({ table }) => (
        <FleetTopToolbar
          table={table}
          availableColumns={mrtColumnManager.allColumns}
          visibleColumnIds={mrtColumnManager.columns
            .filter((col) => {
              const id = getColumnId(col);
              return id && mrtColumnManager.columnVisibility[id];
            })
            .map((col) => getColumnId(col))}
          onToggleColumn={mrtColumnManager.onToggleColumn}
          selectOnlyColumn={mrtColumnManager.selectOnlyColumn}
          resetDefaultColumns={mrtColumnManager.resetDefaultColumns}
        />
      ),
      renderBottomToolbarCustomActions: ({ table }) => (
        <FleetBottomToolbar
          table={table}
          totalSize={repairMetricsList.data?.totalSize}
          nextPageToken={repairMetricsList.data?.nextPageToken}
          pagerCtx={pagerCtx}
        />
      ),
      state: {
        sorting,
        columnVisibility: mrtColumnManager.columnVisibility,
        showProgressBars:
          repairMetricsList.isPending || repairMetricsList.isPlaceholderData,
        columnOrder: [
          'mrt-row-select',
          ...mrtColumnManager.columns
            .filter((col) => {
              const id = getColumnId(col);
              return id && mrtColumnManager.columnVisibility[id];
            })
            .map((col) => getColumnId(col)),
        ],
      },
      manualSorting: true,
      manualPagination: true,
      onColumnVisibilityChange: mrtColumnManager.setColumnVisibility,
    }),
    [
      mrtColumnManager,
      data,
      filterValues,
      repairMetricsList.error,
      repairMetricsList.isPending,
      repairMetricsList.isPlaceholderData,
      repairMetricsList.data?.totalSize,
      repairMetricsList.data?.nextPageToken,
      onSortingChange,
      pagerCtx,
      sorting,
    ],
  );

  const table = useFCDataTable(tableOptions);

  return (
    <>
      <MaterialReactTable table={table} />
    </>
  );
};
