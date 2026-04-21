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

import { ViewColumnOutlined } from '@mui/icons-material';
import { Button, colors, Stack } from '@mui/material';
import { MRT_RowData, MRT_TableInstance } from 'material-react-table';
import * as React from 'react';

import { ColumnsButton } from '@/fleet/components/columns/columns_button';
import { FCDataTableCopy } from '@/fleet/components/fc_data_table/fc_data_table_copy';

import { useFleetTableMeta } from './types';

interface FleetTopToolbarProps<TData extends MRT_RowData> {
  table: MRT_TableInstance<TData>;
  children?: React.ReactNode;
}

export function FleetTopToolbar<TData extends MRT_RowData>({
  table,
  children,
}: FleetTopToolbarProps<TData>) {
  const meta = useFleetTableMeta(table);

  const {
    allDimensionColumns = [],
    visibleColumnIds = [],
    onToggleColumn = () => {},
    selectOnlyColumn = () => {},
    resetDefaultColumns = () => {},
  } = meta;

  return (
    <Stack
      direction="row"
      spacing={1}
      sx={{
        width: '100%',
        justifyContent: 'space-between',
        alignItems: 'center',
      }}
    >
      <Stack direction="row" spacing={1} alignItems="center">
        <FCDataTableCopy table={table} />
        {children}
      </Stack>
      <Stack direction="row" spacing={1} alignItems="center">
        <ColumnsButton
          allColumns={allDimensionColumns}
          visibleColumns={visibleColumnIds}
          onToggleColumn={onToggleColumn}
          selectOnlyColumn={selectOnlyColumn}
          resetDefaultColumns={resetDefaultColumns}
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
      </Stack>
    </Stack>
  );
}
