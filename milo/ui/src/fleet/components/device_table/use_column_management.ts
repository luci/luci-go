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
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { getFilters } from '../filter_dropdown/search_param_utils/search_param_utils';

import { orderColumns } from './columns';
import { useParamsAndLocalStorage } from './use_params_and_local_storage';

/**
 * Styling object for columns that are temporarily visible due to an active filter.
 * This is applied to the DataGrid via the `sx` prop.
 */
const temporaryColumnSx = {
  '& .MuiDataGrid-columnHeader.temp-visible-column, & .MuiDataGrid-cell.temp-visible-column':
    {
      color: colors.blue[600],
      backgroundColor: colors.blue[50],
    },
  '& .MuiDataGrid-columnHeader.temp-visible-column:hover, & .MuiDataGrid-cell.temp-visible-column:hover':
    {
      backgroundColor: colors.blue[100],
    },
};

interface ColumnManagementConfig {
  readonly allColumns: readonly GridColDef[];
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
  defaultColumns,
  localStorageKey,
  preserveOrder = false,
}: ColumnManagementConfig) {
  const [searchParams] = useSyncedSearchParams();

  // Manages columns the user has explicitly chosen to see.
  // This state is persisted in local storage and the URL.
  const [userVisibleColumns, setUserVisibleColumns] = useParamsAndLocalStorage(
    COLUMNS_PARAM_KEY,
    localStorageKey,
    [...defaultColumns],
  );

  // Determines which columns have active filters from the URL search parameters.
  const activeFilterFields = useMemo(() => {
    const filters = getFilters(searchParams).filters || {};
    return new Set(
      Object.keys(filters).map((key) => key.replace('labels.', '')),
    );
  }, [searchParams]);

  // Identifies columns that should be temporarily visible because they have an
  // active filter but are not in the user's set of visible columns.
  const temporaryColumnFields = useMemo(
    () =>
      new Set(
        [...activeFilterFields].filter(
          (field) => !userVisibleColumns.includes(field),
        ),
      ),
    [activeFilterFields, userVisibleColumns],
  );

  // The final set of columns to be displayed, combining user-selected and temporary columns.
  const visibleColumns = useMemo(
    () => [...userVisibleColumns, ...temporaryColumnFields],
    [userVisibleColumns, temporaryColumnFields],
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
      if (newModel[field] && temporaryColumnFields.has(field)) {
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
      if (temporaryColumnFields.has(colDef.field)) {
        return {
          ...colDef,
          hideable: false,
          headerClassName: 'temp-visible-column',
          cellClassName: 'temp-visible-column',
        };
      }
      return colDef;
    });
    return preserveOrder
      ? styledColumns
      : orderColumns(styledColumns, visibleColumns);
  }, [allColumns, visibleColumns, temporaryColumnFields, preserveOrder]);

  const resetDefaultColumns = () => setUserVisibleColumns([...defaultColumns]);
  return {
    columns,
    columnVisibilityModel,
    onColumnVisibilityModelChange,
    resetDefaultColumns,
    temporaryColumnSx,
  };
}
