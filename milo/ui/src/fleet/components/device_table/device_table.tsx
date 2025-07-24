// Copyright 2024 The LUCI Authors.
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

import { Alert } from '@mui/material';
import {
  GridRowModel,
  GridRowSelectionModel,
  GridSortModel,
} from '@mui/x-data-grid';
import { useState, useMemo } from 'react';

import {
  getPageSize,
  PagerContext,
  emptyPageTokenUpdater,
  getCurrentPageIndex,
} from '@/common/components/params_pager';
import { StyledGrid } from '@/fleet/components/styled_data_grid';
import { DEFAULT_DEVICE_COLUMNS } from '@/fleet/config/device_config';
import { DEVICES_COLUMNS_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { extractDutId } from '@/fleet/utils/devices';
import { getErrorMessage } from '@/fleet/utils/errors';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Device } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { CopySnackbar } from '../actions/copy/copy_snackbar';

import { ColumnMenu } from './column_menu';
import {
  BASE_DIMENSIONS,
  COLUMN_OVERRIDES,
  labelValuesToString,
} from './dimensions';
import { FleetToolbar, FleetToolbarProps } from './fleet_toolbar';
import { Pagination } from './pagination';
import { useColumnManagement } from './use_column_management';

const UNKNOWN_ROW_COUNT = -1;

// Used to get around TypeScript issues with custom toolbars.
// See: https://mui.com/x/react-data-grid/components/?srsltid=AfmBOoqlDTexbfxLLrstTWIEaJ97nrqXGVqhaMHF3Q2yIujjoMRTtTvF#custom-slot-props-with-typescript
declare module '@mui/x-data-grid' {
  interface ToolbarPropsOverrides extends FleetToolbarProps {}
}

const computeSelectedRows = (
  gridSelection: GridRowSelectionModel,
  rows: GridRowModel[],
): GridRowModel[] => {
  const selectedSet = new Set(gridSelection);
  return rows.filter((r) => selectedSet.has(r.id));
};

const getRow = (device: Device) =>
  Object.fromEntries<string>([
    ...Object.entries(BASE_DIMENSIONS).map<[string, string]>(([id, dim]) => [
      id,
      dim.getValue?.(device) ?? labelValuesToString([id]),
    ]),
    ...Object.entries(device.deviceSpec?.labels ?? {}).map<[string, string]>(
      ([label, { values }]) => [
        label,
        COLUMN_OVERRIDES[label]?.getValue?.(device) ??
          labelValuesToString(values),
      ],
    ),
  ]);

const getOrderByFromSortModel = (sortModel: GridSortModel): string => {
  if (sortModel.length !== 1) {
    return '';
  }

  const sortItem = sortModel[0];
  const sortKey = BASE_DIMENSIONS[sortItem.field]
    ? sortItem.field
    : `labels.${sortItem.field}`;
  return sortItem.sort === 'desc' ? `${sortKey} desc` : sortKey;
};

interface DeviceTableProps {
  devices: readonly Device[];
  columnIds: string[];
  nextPageToken: string;
  pagerCtx: PagerContext;
  isError: boolean;
  error: unknown;
  isLoading: boolean;
  isLoadingColumns: boolean;
  totalRowCount?: number;
  currentTaskMap: Map<string, string>;
}

export function DeviceTable({
  devices,
  columnIds,
  nextPageToken,
  pagerCtx,
  isError,
  error,
  isLoading,
  isLoadingColumns,
  totalRowCount,
  currentTaskMap,
}: DeviceTableProps) {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [sortModel, setSortModel] = useState<GridSortModel>([]);
  const [, setOrderByParam] = useOrderByParam();
  const [rowSelectionModel, setRowSelectionModel] =
    useState<GridRowSelectionModel>([]);
  const [showCopySuccess, setShowCopySuccess] = useState(false);

  const {
    columns,
    columnVisibilityModel,
    onColumnVisibilityModelChange,
    resetDefaultColumns,
    temporaryColumnSx,
  } = useColumnManagement({
    allColumnIds: columnIds,
    defaultColumns: DEFAULT_DEVICE_COLUMNS,
    localStorageKey: DEVICES_COLUMNS_LOCAL_STORAGE_KEY,
  });

  const rows = useMemo(
    () =>
      devices.map((d) => ({
        ...getRow(d),
        current_task: currentTaskMap.get(extractDutId(d)) || '',
      })),
    [devices, currentTaskMap],
  );

  const onSortModelChange = (newSortModel: GridSortModel) => {
    // Update order by param and clear pagination token when the sort model changes.
    setSortModel(newSortModel);
    setOrderByParam(getOrderByFromSortModel(newSortModel));
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  };

  if (isError) {
    return (
      <Alert severity="error">
        Something went wrong: {getErrorMessage(error, 'list devices')}
      </Alert>
    );
  }

  return (
    <>
      <StyledGrid
        sx={temporaryColumnSx}
        slots={{
          pagination: Pagination,
          columnMenu: ColumnMenu,
          toolbar: FleetToolbar,
        }}
        slotProps={{
          pagination: {
            pagerCtx: pagerCtx,
            nextPageToken: nextPageToken,
            totalRowCount: totalRowCount,
          },
          toolbar: {
            selectedRows: computeSelectedRows(rowSelectionModel, rows),
            isLoadingColumns: isLoadingColumns,
            resetDefaultColumns: resetDefaultColumns,
          },
        }}
        disableRowSelectionOnClick
        checkboxSelection
        onRowSelectionModelChange={(newRowSelectionModel) => {
          setRowSelectionModel(newRowSelectionModel);
        }}
        rowSelectionModel={rowSelectionModel}
        sortModel={sortModel}
        onSortModelChange={onSortModelChange}
        rowCount={UNKNOWN_ROW_COUNT}
        sortingMode="server"
        paginationMode="server"
        pageSizeOptions={pagerCtx.options.pageSizeOptions}
        paginationModel={{
          page: getCurrentPageIndex(pagerCtx),
          pageSize: getPageSize(pagerCtx, searchParams),
        }}
        columnVisibilityModel={columnVisibilityModel}
        onColumnVisibilityModelChange={onColumnVisibilityModelChange}
        rows={rows}
        columns={columns}
        loading={isLoading}
        onClipboardCopy={() => setShowCopySuccess(true)}
      />
      <CopySnackbar
        open={showCopySuccess}
        onClose={() => setShowCopySuccess(false)}
      />
    </>
  );
}
