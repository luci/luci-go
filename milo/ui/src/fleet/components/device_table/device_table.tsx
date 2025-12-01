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
  GridColDef,
  GridRowModel,
  GridRowSelectionModel,
  GridSortModel,
  GridValidRowModel,
} from '@mui/x-data-grid';
import { useState } from 'react';

import {
  emptyPageTokenUpdater,
  getCurrentPageIndex,
  getPageSize,
  PagerContext,
} from '@/common/components/params_pager';
import { StyledGrid } from '@/fleet/components/styled_data_grid';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { usePlatform } from '@/fleet/hooks/usePlatform';
import { getErrorMessage } from '@/fleet/utils/errors';
import { InvalidPageTokenAlert } from '@/fleet/utils/invalid-page-token-alert';
import { parseOrderByParam } from '@/fleet/utils/search_param';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { CopySnackbar } from '../actions/copy/copy_snackbar';
import { useColumnManagement } from '../columns/use_column_management';
import { getFilters } from '../filter_dropdown/search_param_utils';

import { ColumnMenu, ColumnMenuProps } from './column_menu';
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
  rows: readonly GridRowModel[],
): GridRowModel[] => {
  const selectedSet = new Set(gridSelection);
  return rows.filter((r) => selectedSet.has(r.id));
};

const getRowId = (platform: Platform) => {
  type DeviceRowModel = GridRowModel & {
    id: string;
  };
  switch (platform) {
    case Platform.ANDROID:
      type AndroidRowModel = DeviceRowModel & {
        fc_machine_type: string;
      };
      return (row: AndroidRowModel) => row.id + (row.fc_machine_type ?? '');
    case Platform.CHROMEOS:
    case Platform.CHROMIUM:
    case Platform.UNSPECIFIED:
      return (row: DeviceRowModel) => row.id;
  }
};

const getOrderByFromSortModel = <R extends GridValidRowModel>(
  sortModel: GridSortModel,
  availableColumns: DeviceTableGridColDef<R>[],
): string => {
  if (sortModel.length !== 1) {
    return '';
  }

  const sortItem = sortModel[0];

  const sortKey = availableColumns.find(
    (c) => c.field === sortItem.field,
  )?.orderByField;
  if (sortKey === undefined) throw Error(`Can not sort by ${sortItem.field}`);

  return sortItem.sort === 'desc' ? `${sortKey} desc` : sortKey;
};

const getSortModelFromOrderBy = (orderByValue: string): GridSortModel => {
  const orderBy = parseOrderByParam(orderByValue);
  return orderBy
    ? [
        {
          field: orderBy.field.replace(/labels\."?(.*?)"?$/, '$1'),
          sort: orderBy.direction,
        },
      ]
    : [];
};

export type DeviceTableGridColDef<R extends GridValidRowModel> =
  GridColDef<R> & {
    orderByField: string;
  };

interface DeviceTableProps<R extends GridValidRowModel> {
  rows: readonly R[];
  availableColumns: DeviceTableGridColDef<R>[];
  defaultColumnIds: string[];
  localStorageKey: string;
  nextPageToken: string;
  pagerCtx: PagerContext;
  isError: boolean;
  error: Error | null;
  isLoading: boolean;
  isLoadingColumns: boolean;
  totalRowCount?: number;
}

export function DeviceTable<R extends GridValidRowModel>({
  rows,
  availableColumns,
  defaultColumnIds,
  localStorageKey,
  nextPageToken,
  pagerCtx,
  isError,
  error,
  isLoading,
  isLoadingColumns,
  totalRowCount,
}: DeviceTableProps<R>) {
  const { platform } = usePlatform();
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [orderByParam, setOrderByParam] = useOrderByParam();
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
    allColumns: availableColumns,
    highlightedColumnIds: getFilteredColumnIds(),
    defaultColumns: defaultColumnIds,
    localStorageKey: localStorageKey,
    platform: platform,
  });

  const onSortModelChange = (newSortModel: GridSortModel) => {
    // Update order by param and clear pagination token when the sort model changes.
    setOrderByParam(getOrderByFromSortModel(newSortModel, availableColumns));
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  };

  if (isError) {
    if (error?.message.includes('invalid_page_token'))
      return (
        <InvalidPageTokenAlert
          pagerCtx={pagerCtx}
          setSearchParams={setSearchParams}
        />
      );
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
        sortModel={getSortModelFromOrderBy(orderByParam)}
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
        getRowId={getRowId(platform)}
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
