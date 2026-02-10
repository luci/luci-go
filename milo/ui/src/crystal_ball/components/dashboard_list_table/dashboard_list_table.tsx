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
import Typography from '@mui/material/Typography';
import { DateTime } from 'luxon';
import {
  MaterialReactTable,
  useMaterialReactTable,
  type MRT_ColumnDef,
  type MRT_Row,
  type MRT_Cell,
} from 'material-react-table';
import { useMemo } from 'react';

import { Dashboard } from '@/crystal_ball/pages/landing_page/types';

interface DashboardListTableProps {
  /**
   * List of dashboards to display in the table.
   */
  dashboards: Dashboard[];
  /**
   * Callback fired when a dashboard row is clicked.
   */
  onDashboardClick?: (dashboard: Dashboard) => void;
}

export function DashboardListTable({
  dashboards,
  onDashboardClick,
}: DashboardListTableProps) {
  const columns = useMemo<MRT_ColumnDef<Dashboard>[]>(
    () => [
      {
        accessorKey: 'name',
        header: 'Name',
        muiTableHeadCellProps: { sx: { width: '100%' } },
        muiTableBodyCellProps: { sx: { width: '100%' } },
        Cell: ({ row }: { row: MRT_Row<Dashboard> }) => (
          <Box>
            <Typography variant="subtitle1" fontWeight="bold">
              {row.original.name}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              {row.original.description}
            </Typography>
          </Box>
        ),
      },
      {
        accessorKey: 'lastModified',
        header: 'Last Modified',
        muiTableHeadCellProps: { sx: { whiteSpace: 'nowrap', width: 'auto' } },
        muiTableBodyCellProps: { sx: { whiteSpace: 'nowrap', width: 'auto' } },
        Cell: ({ cell }: { cell: MRT_Cell<Dashboard> }) => (
          <Typography variant="body2" color="text.secondary">
            {DateTime.fromISO(cell.getValue<string>()).toRelative()}
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
    enablePagination: true,
    enableSorting: true,
    muiTablePaperProps: {
      elevation: 0,
      sx: {
        border: (theme) => `1px solid ${theme.palette.divider}`,
      },
    },
    muiTableBodyRowProps: ({ row }) => ({
      onClick: () => onDashboardClick?.(row.original),
      sx: {
        cursor: onDashboardClick ? 'pointer' : 'default',
      },
    }),
    initialState: {
      pagination: { pageSize: 10, pageIndex: 0 },
      showGlobalFilter: true,
    },
    muiSearchTextFieldProps: {
      variant: 'outlined',
      size: 'small',
      placeholder: 'Search dashboards...',
    },
    enableGlobalFilter: true,
    positionGlobalFilter: 'left',
  });

  return <MaterialReactTable table={table} />;
}
