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

import { GridColDef, GridColumnVisibilityModel } from '@mui/x-data-grid';
import { useMemo } from 'react';

import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { colors } from '@/fleet/theme/colors';

import { orderColumns } from '../device_table/columns';

import { useParamsAndLocalStorage } from './use_params_and_local_storage';

const highlightedColumnClassName = 'column-highlight';

/**
 * Styling object for columns that are highlighted and temporarily visible due to an active filter.
 * This is applied to the DataGrid via the `sx` prop.
 */
const columnHighlightSx = {
  [`& .MuiDataGrid-columnHeader.${highlightedColumnClassName}, & .MuiDataGrid-cell.${highlightedColumnClassName}`]:
    {
      color: colors.blue[600],
      backgroundColor: colors.blue[50],
    },
  [`& .MuiDataGrid-columnHeader.${highlightedColumnClassName}:hover, & .MuiDataGrid-cell.${highlightedColumnClassName}:hover`]:
    {
      backgroundColor: colors.blue[100],
    },
};

export interface ColumnManagementConfig {
  readonly allColumns: readonly GridColDef[];
  readonly highlightedColumnIds: readonly string[];
  readonly defaultColumns: readonly string[];
  readonly localStorageKey: string;
  readonly preserveOrder?: boolean;
}

/**
 * A hook to manage the state of DataGrid columns, including visibility,
 * persistence, and temporary display based on active filters.
 *
 * It syncs the user's preferred visible columns with URL parameters and local storage.
 * If a filter is applied to a column that is not currently visible, this hook
 * will make that column temporarily visible and apply a distinct style to it.
 */
export function useColumnManagement({
  allColumns,
  highlightedColumnIds,
  defaultColumns,
  localStorageKey,
  preserveOrder = false,
}: ColumnManagementConfig) {
  // Manages columns the user has explicitly chosen to see.
  // This state is persisted in local storage and the URL.
  const [userVisibleColumns, setUserVisibleColumns] = useParamsAndLocalStorage(
    COLUMNS_PARAM_KEY,
    localStorageKey,
    [...defaultColumns],
  );

  const temporaryColumnIds = useMemo(
    () =>
      highlightedColumnIds.filter((col) => !userVisibleColumns.includes(col)),
    [highlightedColumnIds, userVisibleColumns],
  );

  // The final set of columns to be displayed, combining user-selected and temporary columns.
  const visibleColumns = useMemo(
    () => [...userVisibleColumns, ...temporaryColumnIds],
    [userVisibleColumns, temporaryColumnIds],
  );

  // The visibility model required by the MUI DataGrid.
  const columnVisibilityModel = useMemo(() => {
    const model: GridColumnVisibilityModel = {};
    allColumns.forEach(
      (col) => (model[col.field] = visibleColumns.includes(col.field)),
    );
    return model;
  }, [allColumns, visibleColumns]);

  // Callback for when the user changes column visibility in the UI.
  // It ensures that temporary columns cannot be hidden by the user.
  const onColumnVisibilityModelChange = (
    newModel: GridColumnVisibilityModel,
  ) => {
    const newVisible = Object.keys(newModel).filter((field) => {
      // Prevent temporary columns from being saved to the user's preferences.
      if (temporaryColumnIds.includes(field)) {
        return false;
      }
      return newModel[field];
    });
    setUserVisibleColumns(newVisible);
  };

  // Generates the final column definitions for the DataGrid.
  // It adds special properties (e.g., class names) to temporary columns.
  const columns = useMemo(() => {
    const styledColumns = allColumns.map((colDef) => {
      if (highlightedColumnIds.includes(colDef.field)) {
        return {
          ...colDef,
          hideable: false,
          headerClassName: highlightedColumnClassName,
          cellClassName: highlightedColumnClassName,
        };
      }
      return colDef;
    });
    return preserveOrder
      ? styledColumns
      : orderColumns(styledColumns, visibleColumns);
  }, [allColumns, visibleColumns, highlightedColumnIds, preserveOrder]);

  const resetDefaultColumns = () => setUserVisibleColumns([...defaultColumns]);
  return {
    columns,
    columnVisibilityModel,
    onColumnVisibilityModelChange,
    resetDefaultColumns,
    temporaryColumnSx: columnHighlightSx,
  };
}
