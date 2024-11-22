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
  GridSlots,
  GridToolbarColumnsButton,
  GridToolbarContainer,
  useGridApiRef,
  GridPaginationModel,
} from '@mui/x-data-grid';
import * as React from 'react';

const UNKNOWN_ROW_COUNT = -1;
const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50];

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

export const DataTable = ({
  columns,
  rows,
  nextPageToken,
  isLoading,
  paginationModel,
  onPaginationModelChange,
}: {
  columns: GridColDef[];
  rows: { [key: string]: string }[];
  nextPageToken: string;
  isLoading: boolean;
  paginationModel: GridPaginationModel;
  onPaginationModelChange: (model: GridPaginationModel) => void;
}) => {
  const apiRef = useGridApiRef();
  const [columnsButtonEl, setColumnsButtonEl] =
    React.useState<HTMLButtonElement | null>(null);

  // This is a way to autosize columns to fit its content,
  // TODO(vaghinak): call autosizeColumns when data is fetched from backend
  React.useEffect(() => {
    apiRef.current?.autosizeColumns();
  });

  // On the last page we will have the total number of rows
  const rowCount =
    paginationModel.page * paginationModel.pageSize + rows.length;
  const hasNextPage = nextPageToken !== '';
  const isLastPage = !isLoading && !hasNextPage;

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
      rowCount={isLastPage ? rowCount : UNKNOWN_ROW_COUNT}
      paginationMode="server"
      pageSizeOptions={DEFAULT_PAGE_SIZE_OPTIONS}
      paginationMeta={{ hasNextPage: hasNextPage }}
      paginationModel={paginationModel}
      onPaginationModelChange={onPaginationModelChange}
      loading={isLoading}
      rows={rows}
      columns={columns}
    />
  );
};
