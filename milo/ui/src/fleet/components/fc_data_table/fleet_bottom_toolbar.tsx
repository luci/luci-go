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

import { Box, TablePagination } from '@mui/material';
import { MRT_RowData, MRT_TableInstance } from 'material-react-table';
import * as React from 'react';

import { PagerContext } from '@/common/components/params_pager/context';
import { usePager } from '@/fleet/hooks/use_pager';

export const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100, 500, 1000];

interface FleetBottomToolbarProps<TData extends MRT_RowData> {
  table: MRT_TableInstance<TData>;
  totalSize?: number;
  nextPageToken?: string;
  pagerCtx: PagerContext;
}

export function FleetBottomToolbar<TData extends MRT_RowData>({
  table,
  totalSize,
  nextPageToken,
  pagerCtx,
}: FleetBottomToolbarProps<TData>) {
  const {
    pageSize,
    pageIndex: currentPage,
    goToNextPage,
    goToPrevPage,
    onRowsPerPageChange,
  } = usePager(pagerCtx);
  const data = table.options.data || [];

  // Extract condition to improve readability per review feedback
  const isTableEmpty = totalSize === 0 && !nextPageToken && !data.length;

  return (
    <Box sx={{ width: '100%', display: 'flex', justifyContent: 'flex-end' }}>
      <TablePagination
        component="div"
        // Passing -1 to count denotes that the total count is unknown,
        // which enables the "next" button without showing a total page count.
        count={isTableEmpty ? 0 : -1}
        page={currentPage}
        rowsPerPage={pageSize}
        onPageChange={(_, page) => {
          const isPrevPage = page < currentPage;
          const isNextPage = page > currentPage;

          if (isPrevPage) {
            goToPrevPage();
          } else if (isNextPage) {
            goToNextPage(nextPageToken || '');
          }
        }}
        onRowsPerPageChange={(e) => {
          onRowsPerPageChange(Number(e.target.value));
        }}
        rowsPerPageOptions={DEFAULT_PAGE_SIZE_OPTIONS}
        labelDisplayedRows={({ from, to }) => {
          if (totalSize !== undefined && totalSize > 0) {
            return `${from}-${to} of ${totalSize}`;
          }
          return `${from}-${to} of ${nextPageToken ? `more than ${to}` : to}`;
        }}
        slotProps={{
          actions: {
            previousButtonProps: {
              disabled: currentPage === 0,
            },
            nextButtonProps: {
              disabled: data.length === 0 || nextPageToken === '',
            },
          } as NonNullable<
            React.ComponentProps<typeof TablePagination>['slotProps']
          >['actions'],
        }}
      />
    </Box>
  );
}
