// Copyright 2025 The LUCI Authors.
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

import { Box } from '@mui/material';
import {
  GridToolbarContainer,
  GridToolbarDensitySelector,
  gridColumnDefinitionsSelector,
  gridColumnVisibilityModelSelector,
  useGridApiContext,
} from '@mui/x-data-grid';
import { startTransition } from 'react';

import { ColumnsButton } from '@/fleet/components/columns/columns_button';

/**
 * Custom data table toolbar styled for Fleet's expected usage.
 */
export function RriTableToolbar({
  resetDefaultColumns,
}: {
  resetDefaultColumns?: () => void;
}) {
  const apiRef = useGridApiContext();
  const columnVisibilityModel = gridColumnVisibilityModelSelector(apiRef);
  const columnDefinitions = gridColumnDefinitionsSelector(apiRef);

  const onToggleColumn = (field: string) => {
    if (!columnVisibilityModel) return;
    startTransition(() => {
      apiRef.current.setColumnVisibility(field, !columnVisibilityModel[field]);
    });
  };

  const allColumns = columnDefinitions
    .filter((column) => column.field !== '__check__')
    .map((d) => ({ id: d.field, label: d.headerName || d.field }));

  const visibleColumns = columnVisibilityModel
    ? Object.keys(columnVisibilityModel).filter(
        (key) => columnVisibilityModel[key],
      )
    : [];

  return (
    <GridToolbarContainer>
      <Box sx={{ flexGrow: 1 }} />
      <GridToolbarDensitySelector />
      <ColumnsButton
        resetDefaultColumns={resetDefaultColumns}
        allColumns={allColumns}
        visibleColumns={visibleColumns}
        onToggleColumn={onToggleColumn}
      />
    </GridToolbarContainer>
  );
}
