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

import {
  MRT_ColumnDef,
  MRT_VisibilityState,
  MRT_Updater,
  MRT_RowData,
} from 'material-react-table';
import { useMemo, useCallback } from 'react';

import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';

import { useParamsAndLocalStorage } from './use_params_and_local_storage';

export const highlightedColumnClassName = 'column-highlight';
export const temporaryColumnClassName = `${highlightedColumnClassName} temporary-column-highlight`;

export const getColumnId = <TData extends Record<string, unknown>>(
  col: MRT_ColumnDef<TData>,
): string => {
  return col.id || col.accessorKey?.toString() || '';
};

export interface UseMRTColumnManagementProps<TData extends MRT_RowData> {
  columns: MRT_ColumnDef<TData>[];
  // IDs of columns that should be visible by default
  defaultColumnIds: string[];
  // Key for local storage persistence
  localStorageKey: string;
  // IDs of columns that are currently filtered and should be highlighted
  highlightedColumnIds?: readonly string[];
}

export function useMRTColumnManagement<TData extends MRT_RowData>({
  columns: rawColumns,
  defaultColumnIds,
  localStorageKey,
  highlightedColumnIds = [],
}: UseMRTColumnManagementProps<TData>) {
  const [visibleColumnIds, setVisibleColumnIds] = useParamsAndLocalStorage(
    COLUMNS_PARAM_KEY,
    localStorageKey,
    defaultColumnIds,
  );

  const temporaryColumnIds = useMemo(
    () => highlightedColumnIds.filter((col) => !visibleColumnIds.includes(col)),
    [highlightedColumnIds, visibleColumnIds],
  );

  const columnVisibility = useMemo(() => {
    const visibility: MRT_VisibilityState = {};
    rawColumns.forEach((col) => {
      const id = getColumnId(col);
      if (id) {
        visibility[id] =
          visibleColumnIds.includes(id) || temporaryColumnIds.includes(id);
      }
    });
    return visibility;
  }, [rawColumns, visibleColumnIds, temporaryColumnIds]);

  const setColumnVisibility = useCallback(
    (updaterOrValue: MRT_Updater<MRT_VisibilityState>) => {
      let newVisibility: MRT_VisibilityState;
      if (typeof updaterOrValue === 'function') {
        newVisibility = updaterOrValue(columnVisibility);
      } else {
        newVisibility = updaterOrValue;
      }

      const newVisibleIds = Object.keys(newVisibility).filter((id) =>
        temporaryColumnIds.includes(id) ? false : newVisibility[id],
      );
      setVisibleColumnIds(newVisibleIds);
    },
    [columnVisibility, setVisibleColumnIds, temporaryColumnIds],
  );

  const onToggleColumn = useCallback(
    (columnId: string) => {
      if (temporaryColumnIds.includes(columnId)) {
        // Toggling a temporary column makes it permanent
        setVisibleColumnIds([...visibleColumnIds, columnId]);
        return;
      }

      const isVisible = visibleColumnIds.includes(columnId);
      if (isVisible) {
        setVisibleColumnIds(visibleColumnIds.filter((id) => id !== columnId));
      } else {
        setVisibleColumnIds([...visibleColumnIds, columnId]);
      }
    },
    [visibleColumnIds, setVisibleColumnIds, temporaryColumnIds],
  );

  const allColumns = useMemo(
    () =>
      rawColumns.map((c) => ({
        id: getColumnId(c),
        label: c.header || getColumnId(c),
      })),
    [rawColumns],
  );

  const columns = useMemo(() => {
    return rawColumns.map((colDef) => {
      const colId = getColumnId(colDef);
      if (!colId) return colDef;

      let className = '';
      let isTemp = false;

      if (temporaryColumnIds.includes(colId)) {
        className = temporaryColumnClassName;
        isTemp = true;
      } else if (highlightedColumnIds.includes(colId)) {
        className = highlightedColumnClassName;
      }

      if (className || isTemp) {
        return {
          ...colDef,
          enableHiding: isTemp ? false : colDef.enableHiding,
          meta: {
            ...colDef.meta,
            isTemporary: isTemp,
          },
          muiTableHeadCellProps: {
            ...colDef.muiTableHeadCellProps,
            className:
              [
                (
                  colDef.muiTableHeadCellProps as
                    | { className?: string }
                    | undefined
                )?.className,
                className,
              ]
                .filter(Boolean)
                .join(' ') || undefined,
          },
          muiTableBodyCellProps: {
            ...colDef.muiTableBodyCellProps,
            className:
              [
                (
                  colDef.muiTableBodyCellProps as
                    | { className?: string }
                    | undefined
                )?.className,
                className,
              ]
                .filter(Boolean)
                .join(' ') || undefined,
          },
        } as MRT_ColumnDef<TData>;
      }
      return colDef;
    });
  }, [rawColumns, temporaryColumnIds, highlightedColumnIds]);

  return {
    columns,
    columnVisibility,
    setColumnVisibility,
    visibleColumnIds,
    onToggleColumn,
    allColumns,
    resetDefaultColumns: () => setVisibleColumnIds([...defaultColumnIds]),
  };
}
