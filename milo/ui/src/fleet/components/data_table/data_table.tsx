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

import Box from '@mui/material/Box';
import {
  DataGrid,
  gridClasses,
  GridColDef,
  GridColumnVisibilityModel,
  GridPaginationModel,
  GridSlots,
  GridSortModel,
  GridToolbarColumnsButton,
  GridToolbarContainer,
  useGridApiRef,
} from '@mui/x-data-grid';
import * as React from 'react';

import {
  getCurrentPageIndex,
  getPageSize,
  nextPageTokenUpdater,
  PagerContext,
  pageSizeUpdater,
  prevPageTokenUpdater,
} from '@/common/components/params_pager';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { visibleColumnsUpdater, getVisibleColumns } from './search_param_utils';

const UNKNOWN_ROW_COUNT = -1;

// This is done for proper typing
// For more info refer to: https://mui.com/x/common-concepts/custom-components/#using-module-augmentation
interface CustomToolbarProps {
  setColumnsButtonEl: (element: HTMLButtonElement | null) => void;
}

function CustomToolbar({ setColumnsButtonEl }: CustomToolbarProps) {
  return (
    <GridToolbarContainer>
      <Box sx={{ flexGrow: 1 }} />
      <GridToolbarColumnsButton ref={setColumnsButtonEl} />
    </GridToolbarContainer>
  );
}

interface DataTableProps {
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
}

export const DataTable = ({
  defaultColumnVisibilityModel,
  columns,
  rows,
  nextPageToken,
  isLoading,
  pagerCtx,
  sortModel,
  onSortModelChange,
}: DataTableProps) => {
  const apiRef = useGridApiRef();
  const [columnsButtonEl, setColumnsButtonEl] =
    React.useState<HTMLButtonElement | null>(null);
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const paginationModel = {
    page: getCurrentPageIndex(pagerCtx),
    pageSize: getPageSize(pagerCtx, searchParams),
  };

  const onPaginationModelChange = (newPaginationModel: GridPaginationModel) => {
    const isPrevPage = newPaginationModel.page < paginationModel.page;
    const isNextPage = newPaginationModel.page > paginationModel.page;

    setSearchParams(pageSizeUpdater(pagerCtx, newPaginationModel.pageSize));

    if (isPrevPage) {
      setSearchParams(prevPageTokenUpdater(pagerCtx));
    } else if (isNextPage) {
      setSearchParams(nextPageTokenUpdater(pagerCtx, nextPageToken));
    }
  };

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

  // This is a way to autosize columns to fit its content,
  // TODO(vaghinak): call autosizeColumns when data is fetched from backend
  React.useEffect(() => {
    apiRef.current?.autosizeColumns();
  });

  const [rowCount, setRowCount] = React.useState(UNKNOWN_ROW_COUNT);
  const hasNextPage = isLoading || nextPageToken !== '';

  return (
    <DataGrid
      apiRef={apiRef}
      sx={{
        [`& .${gridClasses.columnHeader}`]: {
          backgroundColor: '#f2f3f4',
          height: 'unset !important',
          minHeight: 56,
        },
        [`& .${gridClasses.columnSeparator}`]: {
          color: '#f2f3f4',
        },
        [`& .${gridClasses.filler}`]: {
          background: '#f2f3f4',
        },
        [`& .${gridClasses.cell}`]: {
          py: 2,
        },
        [`& .${gridClasses.columnHeaderTitle}`]: {
          whiteSpace: 'normal',
        },
      }}
      slots={{
        toolbar: CustomToolbar as GridSlots['toolbar'],
      }}
      slotProps={{
        panel: {
          anchorEl: columnsButtonEl,
          placement: 'bottom-end',
        },
        columnsManagement: {
          disableShowHideToggle: true,
        },
        toolbar: { setColumnsButtonEl },
      }}
      getRowHeight={() => 'auto'}
      disableRowSelectionOnClick
      disableColumnMenu
      sortModel={sortModel}
      onSortModelChange={onSortModelChange}
      rowCount={rowCount}
      onRowCountChange={(newRowCount) => {
        setRowCount(hasNextPage ? UNKNOWN_ROW_COUNT : newRowCount);
      }}
      paginationMode="server"
      pageSizeOptions={pagerCtx.options.pageSizeOptions}
      paginationMeta={{ hasNextPage: hasNextPage }}
      paginationModel={paginationModel}
      onPaginationModelChange={onPaginationModelChange}
      columnVisibilityModel={getVisibleColumns(
        searchParams,
        defaultColumnVisibilityModel,
        columns,
      )}
      onColumnVisibilityModelChange={onColumnVisibilityModelChange}
      loading={isLoading}
      rows={rows}
      columns={columns}
    />
  );
};
