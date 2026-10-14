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
import { Button, Stack } from '@mui/material';
import { MRT_RowData, MRT_TableInstance } from 'material-react-table';
import * as React from 'react';

import { ColumnsButton } from '@/fleet/components/columns/columns_button';
import { ColumnOption } from '@/fleet/components/columns/use_mrt_column_management';
import { FCDataTableCopy } from '@/fleet/components/fc_data_table/fc_data_table_copy';

interface FleetTopToolbarProps<TData extends MRT_RowData> {
  table: MRT_TableInstance<TData>;
  availableColumns?: ColumnOption[];
  visibleColumnIds?: string[];
  onToggleColumn?: (id: string) => void;
  selectOnlyColumn?: (id: string) => void;
  resetDefaultColumns?: () => void;
  resetColumnWidths?: () => void;
  children?: React.ReactNode;
}

export function FleetTopToolbar<TData extends MRT_RowData>({
  table,
  availableColumns = [],
  visibleColumnIds = [],
  onToggleColumn = () => {},
  selectOnlyColumn = () => {},
  resetDefaultColumns = () => {},
  resetColumnWidths,
  children,
}: FleetTopToolbarProps<TData>) {
  const columnVisibility = table.getState()?.columnVisibility;

  const activeFilteredColumnIds = React.useMemo(
    () =>
      availableColumns
        .filter((col) => col.isTemporary || col.isHighlighted)
        .map((col) => col.id),
    [availableColumns],
  );

  const activeVisibleColumns = React.useMemo(() => {
    if (!columnVisibility || Object.keys(columnVisibility).length === 0) {
      return visibleColumnIds;
    }
    const visibleSet = new Set(visibleColumnIds);
    return availableColumns
      .filter((c) =>
        c.id in columnVisibility
          ? Boolean(columnVisibility[c.id])
          : visibleSet.has(c.id),
      )
      .map((c) => c.id);
  }, [columnVisibility, visibleColumnIds, availableColumns]);

  return (
    <Stack
      direction="row"
      useFlexGap
      sx={{
        width: '100%',
        gap: 1,
        justifyContent: 'space-between',
        alignItems: 'center',
      }}
    >
      <Stack
        direction="row"
        useFlexGap
        alignItems="center"
        sx={{ gap: 1, flexWrap: 'wrap' }}
      >
        <FCDataTableCopy table={table} />
        {children}
      </Stack>
      <Stack direction="row" useFlexGap alignItems="center" sx={{ gap: 1 }}>
        <ColumnsButton
          allColumns={availableColumns}
          visibleColumns={activeVisibleColumns}
          temporaryColumns={activeFilteredColumnIds}
          onToggleColumn={onToggleColumn}
          selectOnlyColumn={selectOnlyColumn}
          resetDefaultColumns={resetDefaultColumns}
          resetColumnWidths={resetColumnWidths}
          renderTrigger={({ onClick }, ref) => (
            <Button
              ref={ref}
              variant="toolbar"
              size="small"
              startIcon={<ViewColumnOutlined />}
              onClick={onClick}
              color="inherit"
            >
              Columns
            </Button>
          )}
        />
      </Stack>
    </Stack>
  );
}
