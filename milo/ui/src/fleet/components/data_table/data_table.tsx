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

import { CircularProgress } from '@mui/material';
import {
  DataGrid,
  GridAutosizeOptions,
  gridClasses,
  GridColDef,
  GridColumnVisibilityModel,
  GridSortModel,
  useGridApiRef,
} from '@mui/x-data-grid';
import _ from 'lodash';
import * as React from 'react';

import {
  getCurrentPageIndex,
  getPageSize,
  PagerContext,
} from '@/common/components/params_pager';
import { colors } from '@/fleet/theme/colors';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { Pagination } from './pagination';
import { visibleColumnsUpdater, getVisibleColumns } from './search_param_utils';
import { Toolbar } from './toolbar';

const UNKNOWN_ROW_COUNT = -1;

const autosizeOptions: GridAutosizeOptions = {
  expand: true,
  includeHeaders: true,
  includeOutliers: true,
};

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

  const onColumnVisibilityModelChange = (
    newColumnVisibilityModel: GridColumnVisibilityModel,
  ) => {
    setSearchParams(
      visibleColumnsUpdater(
        newColumnVisibilityModel,
        defaultColumnVisibilityModel,
      ),
    );
    apiRef.current.autosizeColumns(autosizeOptions);
  };

  React.useEffect(() => {
    const autosize = _.debounce(() => {
      apiRef.current.autosizeColumns(autosizeOptions);
    }, 300);

    window.addEventListener('resize', autosize);
    return () => window.removeEventListener('resize', autosize);
  }, [apiRef]);

  // We avoid rendering the DataGrid while loading so that when autosizeOnMount
  // is fired it already has the correct columns loaded.
  if (isLoading) {
    return (
      <div
        css={{
          width: '100%',
          padding: '0 50%',
        }}
      >
        <CircularProgress />
      </div>
    );
  }

  return (
    <DataGrid
      apiRef={apiRef}
      autosizeOnMount
      autosizeOptions={autosizeOptions}
      sx={{
        [`& .${gridClasses.columnHeader}`]: {
          backgroundColor: colors.grey[100],
          height: 'unset !important',
          minHeight: 56,
        },
        [`& .${gridClasses.columnSeparator}`]: {
          color: colors.grey[100],
        },
        [`& .${gridClasses.filler}`]: {
          background: colors.grey[100],
        },
        [`& .${gridClasses.cell}`]: {
          py: 2,
        },
        [`& .${gridClasses.columnHeaderTitle}`]: {
          whiteSpace: 'normal',
        },
      }}
      slots={{
        toolbar: Toolbar,
        pagination: Pagination,
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
        pagination: {
          pagerCtx: pagerCtx,
          nextPageToken: nextPageToken,
        },
      }}
      getRowHeight={() => 'auto'}
      disableRowSelectionOnClick
      disableColumnMenu
      sortModel={sortModel}
      onSortModelChange={onSortModelChange}
      rowCount={UNKNOWN_ROW_COUNT}
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
    />
  );
};
