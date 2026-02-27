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

import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import MenuItem from '@mui/material/MenuItem';
import Snackbar from '@mui/material/Snackbar';
import Typography from '@mui/material/Typography';
import { useQueryClient } from '@tanstack/react-query';
import {
  MaterialReactTable,
  useMaterialReactTable,
  type MRT_ColumnDef,
  type MRT_Row,
  type MRT_Cell,
} from 'material-react-table';
import { useMemo, useState } from 'react';

import { DeleteDashboardDialog } from '@/crystal_ball/components/dashboard_dialog/delete_dashboard_dialog';
import {
  listDashboardStatesQueryKey,
  useDeleteDashboardState,
  useListDashboardStatesInfinite,
} from '@/crystal_ball/hooks';
import { DashboardState, Timestamp } from '@/crystal_ball/types';
import {
  escapeRegExp,
  formatApiError,
  formatRelativeTime,
} from '@/crystal_ball/utils';

interface DashboardListTableProps {
  /**
   * Callback fired when a dashboard row is clicked.
   */
  onDashboardClick?: (dashboard: DashboardState) => void;
}

const getDisplayName = (dashboard: DashboardState) =>
  dashboard.displayName ||
  dashboard.name?.split('/').pop() ||
  'Unnamed Dashboard';

function useLoadMoreDashboards() {
  const [globalFilter, setGlobalFilter] = useState('');

  const requestParams = useMemo(
    () => ({
      pageSize: 20,
      filter: globalFilter ? escapeRegExp(globalFilter) : '',
    }),
    [globalFilter],
  );

  const queryParams = useListDashboardStatesInfinite(requestParams);

  const dashboards = useMemo(() => {
    return (
      queryParams.data?.pages.flatMap((page) => page.dashboardStates || []) ||
      []
    );
  }, [queryParams.data]);

  return {
    ...queryParams,
    dashboards,
    globalFilter,
    setGlobalFilter,
    handleLoadMore: () => queryParams.fetchNextPage(),
    hasNextPage: queryParams.hasNextPage,
    error: queryParams.error,
    refetch: queryParams.refetch,
  };
}

/**
 * A table of dashboards using a "Load More" pagination pattern.
 */
export function DashboardListTable({
  onDashboardClick,
}: DashboardListTableProps) {
  const {
    dashboards,
    isLoading,
    isError,
    error,
    isFetching,
    globalFilter,
    setGlobalFilter,
    handleLoadMore,
    hasNextPage,
  } = useLoadMoreDashboards();

  const [dashboardToDelete, setDashboardToDelete] =
    useState<DashboardState | null>(null);
  const [toastMessage, setToastMessage] = useState('');

  const queryClient = useQueryClient();
  const { mutateAsync: deleteDashboard, isPending: isDeleting } =
    useDeleteDashboardState();

  const handleDelete = async () => {
    if (!dashboardToDelete?.name) return;
    try {
      await deleteDashboard({ name: dashboardToDelete.name });
      setToastMessage('Dashboard deleted successfully');
      setDashboardToDelete(null);
      queryClient.invalidateQueries({
        queryKey: listDashboardStatesQueryKey(),
      });
    } catch (e) {
      setToastMessage(formatApiError(e, 'Failed to delete dashboard'));
      setDashboardToDelete(null);
    }
  };

  const columns = useMemo<MRT_ColumnDef<DashboardState>[]>(
    () => [
      {
        accessorKey: 'name',
        header: 'Name',
        muiTableHeadCellProps: { sx: { width: '100%' } },
        muiTableBodyCellProps: { sx: { width: '100%' } },
        Cell: (args: { row: MRT_Row<DashboardState> }) => (
          <Box>
            <Typography variant="subtitle1" fontWeight="bold">
              {getDisplayName(args.row.original)}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              {args.row.original.description}
            </Typography>
          </Box>
        ),
      },
      {
        accessorKey: 'updateTime',
        header: 'Last Modified',
        muiTableHeadCellProps: { sx: { whiteSpace: 'nowrap', width: 'auto' } },
        muiTableBodyCellProps: { sx: { whiteSpace: 'nowrap', width: 'auto' } },
        Cell: (args: { cell: MRT_Cell<DashboardState> }) => (
          <Typography variant="body2" color="text.secondary">
            {formatRelativeTime(args.cell.getValue<string | Timestamp>())}
          </Typography>
        ),
      },
    ],
    [],
  );

  const table = useMaterialReactTable({
    columns,
    data: dashboards,
    enableColumnActions: false,
    enableColumnFilters: false,
    enableHiding: false,
    enablePagination: false,
    enableSorting: false,
    enableRowActions: true,
    positionActionsColumn: 'last',
    renderRowActionMenuItems: ({ closeMenu, row }) => [
      <MenuItem
        key="delete"
        onClick={(e) => {
          e.stopPropagation();
          setDashboardToDelete(row.original);
          closeMenu();
        }}
        sx={{ color: 'error.main' }}
      >
        Delete Dashboard
      </MenuItem>,
    ],
    enableBottomToolbar: hasNextPage,
    manualFiltering: true,
    onGlobalFilterChange: setGlobalFilter,
    state: {
      globalFilter,
      isLoading: isLoading && dashboards.length === 0,
      showProgressBars: isFetching,
      showAlertBanner: false,
    },
    renderEmptyRowsFallback: () => (
      <Box
        sx={{
          p: 2,
          textAlign: 'center',
          color: isError ? 'error.main' : 'text.secondary',
        }}
      >
        <Typography>
          {isError ? formatApiError(error) : 'No records to display'}
        </Typography>
      </Box>
    ),
    muiTablePaperProps: {
      elevation: 0,
      sx: { border: (theme) => `1px solid ${theme.palette.divider}` },
    },
    muiTableBodyRowProps: (args: { row: MRT_Row<DashboardState> }) => ({
      onClick: () => onDashboardClick?.(args.row.original),
      sx: { cursor: onDashboardClick ? 'pointer' : 'default' },
    }),
    initialState: {
      showGlobalFilter: true,
    },
    muiSearchTextFieldProps: {
      variant: 'outlined',
      size: 'small',
      placeholder: 'Search dashboards...',
    },
    enableGlobalFilter: true,
    positionGlobalFilter: 'left',
    renderBottomToolbarCustomActions: () => {
      if (!hasNextPage) return null;

      return (
        <Box
          sx={{
            width: '100%',
            display: 'flex',
            justifyContent: 'center',
            p: 1,
          }}
        >
          <Button onClick={handleLoadMore} disabled={isFetching} variant="text">
            {isFetching ? 'Loading...' : 'Load More'}
          </Button>
        </Box>
      );
    },
  });

  return (
    <>
      <MaterialReactTable table={table} />
      <DeleteDashboardDialog
        open={Boolean(dashboardToDelete)}
        onClose={() => setDashboardToDelete(null)}
        onConfirm={handleDelete}
        isDeleting={isDeleting}
        dashboardState={dashboardToDelete}
      />
      <Snackbar
        open={Boolean(toastMessage)}
        autoHideDuration={4000}
        onClose={() => setToastMessage('')}
        message={toastMessage}
      />
    </>
  );
}
