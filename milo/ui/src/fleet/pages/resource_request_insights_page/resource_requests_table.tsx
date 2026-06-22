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

import ViewColumnOutlined from '@mui/icons-material/ViewColumnOutlined';
import { Box, Button, TablePagination } from '@mui/material';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { MaterialReactTable, MRT_PaginationState } from 'material-react-table';
import { useMemo } from 'react';

import {
  nextPageTokenUpdater,
  pageSizeUpdater,
  prevPageTokenUpdater,
  usePagerContext,
} from '@/common/components/params_pager';
import { ColumnsButton } from '@/fleet/components/columns/columns_button';
import { MrtColumnManager } from '@/fleet/components/columns/use_mrt_column_management';
import { FCDataTableCopy } from '@/fleet/components/fc_data_table/fc_data_table_copy';
import {
  FC_ColumnDef,
  useFCDataTable,
} from '@/fleet/components/fc_data_table/use_fc_data_table';
import { PAGE_TOKEN_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useMrtSortingState } from '@/fleet/hooks/use_mrt_sorting_state';
import { usePager } from '@/fleet/hooks/use_pager';
import { colors } from '@/fleet/theme/colors';
import { getErrorMessage } from '@/fleet/utils/errors';
import { InvalidPageTokenAlert } from '@/fleet/utils/invalid-page-token-alert';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { DEFAULT_SORT_COLUMN_ID } from './rri_columns';
import { getRow, type RriGridRow } from './rri_utils';
import { useRriFilters } from './use_rri_filters';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50];
const DEFAULT_PAGE_SIZE = 25;

interface ResourceRequestTableProps {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mrtColumnManager: MrtColumnManager<FC_ColumnDef<RriGridRow, any>>;
}

export const ResourceRequestTable = ({
  mrtColumnManager,
}: ResourceRequestTableProps) => {
  const [, setSearchParams] = useSyncedSearchParams();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
    pageTokenKey: PAGE_TOKEN_PARAM_KEY,
  });

  const { pageSize, pageToken, pageIndex } = usePager(pagerCtx);

  const { filterValues, aipString } = useRriFilters();

  const client = useFleetConsoleClient();

  const [sorting, onSortingChange, orderByParam] = useMrtSortingState(
    mrtColumnManager.columns.map((c) => ({
      id: c.id || (c.accessorKey as string) || '',
    })),
    pagerCtx,
  );

  const pagination = useMemo<MRT_PaginationState>(
    () => ({
      pageIndex,
      pageSize,
    }),
    [pageIndex, pageSize],
  );

  const query = useQuery({
    ...client.ListResourceRequests.query({
      filter: aipString,
      orderBy: orderByParam || `${DEFAULT_SORT_COLUMN_ID} desc`,
      pageSize: pageSize,
      pageToken: pageToken,
    }),
    placeholderData: keepPreviousData,
  });

  const countQuery = useQuery({
    ...client.CountResourceRequests.query({
      filter: aipString,
    }),
  });

  const table = useFCDataTable<RriGridRow, unknown>({
    columns: mrtColumnManager.columns as FC_ColumnDef<RriGridRow, unknown>[],
    enableColumnActions: true,
    manualFiltering: true,
    enableColumnFilters: false,
    filterValues: filterValues,
    error:
      query.error || countQuery.error
        ? getErrorMessage(
            query.error || countQuery.error,
            'get resource requests',
          )
        : undefined,
    enableRowSelection: false,
    renderTopToolbarCustomActions: ({ table }) => (
      <div
        css={{
          width: '100%',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: '8px',
        }}
      >
        <FCDataTableCopy table={table} />
        <ColumnsButton
          allColumns={mrtColumnManager.allColumns}
          visibleColumns={mrtColumnManager.visibleColumnIds}
          onToggleColumn={mrtColumnManager.onToggleColumn}
          selectOnlyColumn={mrtColumnManager.selectOnlyColumn}
          resetDefaultColumns={mrtColumnManager.resetDefaultColumns}
          renderTrigger={({ onClick }, ref) => (
            <Button
              ref={ref}
              startIcon={<ViewColumnOutlined sx={{ fontSize: '20px' }} />}
              onClick={onClick}
              color="inherit"
              sx={{
                color: colors.grey[600],
                height: '40px',
                fontSize: '0.875rem',
                textTransform: 'none',
                fontWeight: 500,
              }}
            >
              Columns
            </Button>
          )}
        />
      </div>
    ),
    data: query.data?.resourceRequests.map((rr) => getRow(rr)) ?? [],
    getRowId: (row) => row.id,
    state: {
      sorting,
      columnVisibility: mrtColumnManager.columnVisibility,
      showProgressBars: query.isPending || query.isPlaceholderData,
      pagination: pagination,
    },
    onColumnVisibilityChange: mrtColumnManager.setColumnVisibility,
    muiTopToolbarProps: {
      sx: {
        '& [aria-label="Show/Hide filters"]': {
          display: 'none',
        },
      },
    },
    manualSorting: true,
    onSortingChange,
    enablePagination: false,
    renderBottomToolbarCustomActions: () => (
      <Box sx={{ width: '100%', display: 'flex', justifyContent: 'flex-end' }}>
        <TablePagination
          component="div"
          count={-1}
          page={pageIndex}
          rowsPerPage={pageSize}
          onPageChange={(_, page) => {
            const isPrevPage = page < pageIndex;
            const isNextPage = page > pageIndex;

            setSearchParams((prev: URLSearchParams) => {
              let next = new URLSearchParams(prev);
              if (isPrevPage) {
                next = prevPageTokenUpdater(pagerCtx)(next);
              } else if (isNextPage) {
                next = nextPageTokenUpdater(
                  pagerCtx,
                  query.data?.nextPageToken ?? '',
                )(next);
              }
              return next;
            });
          }}
          onRowsPerPageChange={(e) => {
            setSearchParams(pageSizeUpdater(pagerCtx, Number(e.target.value)));
          }}
          rowsPerPageOptions={DEFAULT_PAGE_SIZE_OPTIONS}
          labelDisplayedRows={({ from, to }) => {
            if (countQuery.data?.total !== undefined) {
              return `${from}-${to} of ${countQuery.data.total}`;
            }
            return `${from}-${to} of ${
              query.data?.nextPageToken ? `more than ${to}` : to
            }`;
          }}
          slotProps={{
            actions: {
              nextButton: {
                disabled: !query.data?.nextPageToken,
              },
            },
            select: {
              MenuProps: {
                sx: { zIndex: 1401 },
                anchorOrigin: { vertical: 'top', horizontal: 'left' },
                transformOrigin: { vertical: 'bottom', horizontal: 'left' },
              },
            },
          }}
        />
      </Box>
    ),
  });

  if (query.isError && query.error?.message.includes('invalid_page_token')) {
    return (
      <InvalidPageTokenAlert
        pagerCtx={pagerCtx}
        setSearchParams={setSearchParams}
      />
    );
  }

  return (
    <div
      css={{
        borderRadius: 4,
        marginTop: 24,
      }}
    >
      <MaterialReactTable table={table} />
    </div>
  );
};
