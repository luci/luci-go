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
import { useMemo, useState } from 'react';

import {
  emptyPageTokenUpdater,
  getCurrentPageIndex,
  getPageSize,
  PagerContext,
} from '@/common/components/params_pager';
import { StyledGrid } from '@/fleet/components/styled_data_grid';
import { DEFAULT_DEVICE_COLUMNS } from '@/fleet/config/device_config';
import { DEVICES_COLUMNS_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { extractDutId } from '@/fleet/utils/devices';
import { getErrorMessage } from '@/fleet/utils/errors';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  Device,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { CopySnackbar } from '../actions/copy/copy_snackbar';
import { useColumnManagement } from '../columns/use_column_management';
import { getFilters } from '../filter_dropdown/search_param_utils';

import { ColumnMenu, ColumnMenuProps } from './column_menu';
import { getColumns } from './columns';
import {
  BASE_DIMENSIONS,
  COLUMN_OVERRIDES,
  labelValuesToString,
} from './dimensions';
import { FleetToolbar, FleetToolbarProps } from './fleet_toolbar';
import { Pagination } from './pagination';

const UNKNOWN_ROW_COUNT = -1;

// Used to get around TypeScript issues with custom toolbars.
// See: https://mui.com/x/react-data-grid/components/?srsltid=AfmBOoqlDTexbfxLLrstTWIEaJ97nrqXGVqhaMHF3Q2yIujjoMRTtTvF#custom-slot-props-with-typescript
declare module '@mui/x-data-grid' {
  // eslint-disable-next-line @typescript-eslint/no-empty-object-type
  interface ToolbarPropsOverrides extends FleetToolbarProps {}
  // eslint-disable-next-line @typescript-eslint/no-empty-object-type
  interface ColumnMenuPropsOverrides extends ColumnMenuProps {}
}

const computeSelectedRows = (
  gridSelection: GridRowSelectionModel,
  rows: GridRowModel[],
): GridRowModel[] => {
  const selectedSet = new Set(gridSelection);
  return rows.filter((r) => selectedSet.has(r.id));
};

const getRow = (
  platform: Platform,
  currentTaskMap?: Map<string, string>,
): ((device: Device) => GridRowModel) => {
  switch (platform) {
    case Platform.CHROMEOS:
    case Platform.UNSPECIFIED:
      return (device) =>
        Object.fromEntries<string>([
          ...Object.entries(BASE_DIMENSIONS).map(
            ([id, dim]) =>
              [
                id,
                dim.getValue?.(device) ?? labelValuesToString([id]),
              ] as const,
          ),
          ...Object.entries(device.deviceSpec?.labels ?? {}).map(
            ([label, { values }]) =>
              [
                label,
                COLUMN_OVERRIDES[platform][label]?.getValue?.(device) ??
                  labelValuesToString(values),
              ] as const,
          ),
          ['current_task', currentTaskMap?.get(extractDutId(device)) || ''],
        ]);
    case Platform.ANDROID:
      return (device) =>
        Object.fromEntries([
          ['id', device.id],
          ...Object.entries(device.deviceSpec?.labels ?? {}).map(
            ([label, { values }]) =>
              [
                label,
                COLUMN_OVERRIDES[platform][label]?.getValue?.(device) ??
                  labelValuesToString(values),
              ] as const,
          ),
        ]);
    case Platform.CHROMIUM:
      return (_device) => ({}); // TODO;
  }
};

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
  platform: Platform;
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
  platform,
}: DeviceTableProps) {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [sortModel, setSortModel] = useState<GridSortModel>([]);
  const [, setOrderByParam] = useOrderByParam();
  const [rowSelectionModel, setRowSelectionModel] =
    useState<GridRowSelectionModel>([]);
  const [showCopySuccess, setShowCopySuccess] = useState(false);

  const getFilteredColumnIds = () => {
    const filters = getFilters(searchParams)?.filters;
    if (!filters) return [];

    // TODO - b/449694092 this is a temporary fix until we have a streamlined solution for managing filters that puts the logic in one place
    return Object.keys(filters).map((key) =>
      key.replace(/labels\."?(.*?)"?$/, '$1'),
    );
  };

  const {
    columns,
    temporaryColumns,
    columnVisibilityModel,
    onColumnVisibilityModelChange,
    resetDefaultColumns,
    temporaryColumnSx,
    addUserVisibleColumn,
  } = useColumnManagement({
    allColumns: getColumns(columnIds, platform),
    highlightedColumnIds: getFilteredColumnIds(),
    defaultColumns: DEFAULT_DEVICE_COLUMNS[platform],
    localStorageKey: DEVICES_COLUMNS_LOCAL_STORAGE_KEY[platform],
    platform: platform,
  });

  const rows = useMemo(
    () => devices.map(getRow(platform, currentTaskMap)),
    [currentTaskMap, devices, platform],
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
          columnMenu: ColumnMenu,
          pagination: Pagination,
          toolbar: FleetToolbar,
        }}
        slotProps={{
          columnMenu: { platform },
          pagination: {
            pagerCtx: pagerCtx,
            nextPageToken: nextPageToken,
            totalRowCount: totalRowCount,
          },
          toolbar: {
            selectedRows: computeSelectedRows(rowSelectionModel, rows),
            isLoadingColumns: isLoadingColumns,
            resetDefaultColumns: resetDefaultColumns,
            temporaryColumns: temporaryColumns,
            addUserVisibleColumn: addUserVisibleColumn,
            platform,
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
