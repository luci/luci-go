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

import {
  GridColDef,
  GridColumnVisibilityModel,
  GridRowModel,
  GridRowSelectionModel,
  GridSortModel,
} from '@mui/x-data-grid';
import { GridApiCommunity } from '@mui/x-data-grid/internals';
import _ from 'lodash';
import * as React from 'react';

import {
  getCurrentPageIndex,
  getPageSize,
  PagerContext,
} from '@/common/components/params_pager';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { ColumnMenu } from './column_menu';
import { FleetToolbar, FleetToolbarProps } from './fleet_toolbar';
import { Pagination } from './pagination';
import { getVisibleColumns, visibleColumnsUpdater } from './search_param_utils';
import { StyledGrid } from './styled_data_grid';

const UNKNOWN_ROW_COUNT = -1;

const computeSelectedRows = (
  gridSelection: GridRowSelectionModel,
  rows: GridRowModel[],
): GridRowModel[] => {
  const selectedSet = new Set(gridSelection);
  return rows.filter((r) => selectedSet.has(r.id));
};

interface DataTableProps {
  gridRef: React.MutableRefObject<GridApiCommunity>;
  defaultColumnVisibilityModel: GridColumnVisibilityModel;
  columns: GridColDef[];
  rows: {
    [key: string]: string;
  }[];
  nextPageToken: string;
  isLoading: boolean;
  pagerCtx: PagerContext;
  sortModel: GridSortModel;
  onSortModelChange: (newSortModel: GridSortModel) => void;
  totalRowCount?: number;
  isLoadingColumns?: boolean;
}

// Used to get around TypeScript issues with custom toolbars.
// See: https://mui.com/x/react-data-grid/components/?srsltid=AfmBOoqlDTexbfxLLrstTWIEaJ97nrqXGVqhaMHF3Q2yIujjoMRTtTvF#custom-slot-props-with-typescript
declare module '@mui/x-data-grid' {
  interface ToolbarPropsOverrides extends FleetToolbarProps {}
}

// TODO: b/393601163 - Consider combining this directly into Device Table.
export const DataTable = ({
  gridRef: apiRef,
  defaultColumnVisibilityModel,
  columns,
  rows,
  nextPageToken,
  isLoading,
  pagerCtx,
  sortModel,
  onSortModelChange,
  totalRowCount,
  isLoadingColumns,
}: DataTableProps) => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  // See: https://mui.com/x/react-data-grid/row-selection/#controlled-row-selection
  const [rowSelectionModel, setRowSelectionModel] =
    React.useState<GridRowSelectionModel>([]);

  const onColumnVisibilityModelChange = (
    newColumnVisibilityModel: GridColumnVisibilityModel,
  ) => {
    setSearchParams(
      visibleColumnsUpdater(
        newColumnVisibilityModel,
        defaultColumnVisibilityModel,
      ),
    );
  };

  return (
    <StyledGrid
      apiRef={apiRef}
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
          gridRef: apiRef,
          selectedRows: computeSelectedRows(rowSelectionModel, rows),
          isLoadingColumns: isLoadingColumns,
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
      columnVisibilityModel={getVisibleColumns(
        searchParams,
        defaultColumnVisibilityModel,
        columns,
      )}
      onColumnVisibilityModelChange={onColumnVisibilityModelChange}
      rows={rows}
      columns={columns}
      loading={isLoading}
    />
  );
};
