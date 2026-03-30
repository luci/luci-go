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
import { useMemo, useCallback, useState } from 'react';

import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { orderMRTColumns } from '../device_table/columns';

import { useParamsAndLocalStorage } from './use_params_and_local_storage';

export const highlightedColumnClassName = 'column-highlight';
export const temporaryColumnClassName = `${highlightedColumnClassName} temporary-column-highlight`;

export const MRT_INTERNAL_COLUMNS = new Set([
  'mrt-row-select',
  'mrt-row-actions',
  'mrt-row-expand',
  'mrt-row-numbers',
]);

export const sanitizeColumnIds = (ids: string[]) => {
  return ids.filter((id) => !MRT_INTERNAL_COLUMNS.has(id));
};

export const getColumnId = <
  T extends { id?: string; accessorKey?: string | number | symbol },
>(
  col: T,
): string => {
  return col?.id || col?.accessorKey?.toString() || '';
};

export interface UseMRTColumnManagementProps<
  _TData extends MRT_RowData,
  TColumnDef extends {
    id?: string;
    accessorKey?: string | number | symbol;
    header?: unknown;
    meta?: unknown;
    enableHiding?: boolean;
  } = MRT_ColumnDef<_TData>,
> {
  columns: TColumnDef[];
  // IDs of columns that should be visible by default
  defaultColumnIds: string[];
  // Key for local storage persistence
  localStorageKey: string;
  // IDs of columns that are currently filtered and should be highlighted
  highlightedColumnIds?: readonly string[];
  platform?: Platform;
}

export function useMRTColumnManagement<
  _TData extends MRT_RowData,
  TColumnDef extends {
    id?: string;
    accessorKey?: string | number | symbol;
    header?: unknown;
    meta?: unknown;
    enableHiding?: boolean;
  } = MRT_ColumnDef<_TData>,
>({
  columns: rawColumns,
  defaultColumnIds,
  localStorageKey,
  highlightedColumnIds = [],
  platform,
}: UseMRTColumnManagementProps<_TData, TColumnDef>) {
  const [visibleColumnIds, setVisibleColumnIds] = useParamsAndLocalStorage(
    COLUMNS_PARAM_KEY,
    localStorageKey,
    defaultColumnIds,
    sanitizeColumnIds,
  );

  const temporaryColumnIds = useMemo(
    () => highlightedColumnIds.filter((col) => !visibleColumnIds.includes(col)),
    [highlightedColumnIds, visibleColumnIds],
  );

  const [internalVisibility, setInternalVisibility] =
    useState<MRT_VisibilityState>({});

  const columnVisibility = useMemo(() => {
    const visibility: MRT_VisibilityState = { ...internalVisibility };
    rawColumns.forEach((col) => {
      const id = getColumnId(col);
      if (id) {
        visibility[id] =
          visibleColumnIds.includes(id) || temporaryColumnIds.includes(id);
      }
    });
    return visibility;
  }, [rawColumns, visibleColumnIds, temporaryColumnIds, internalVisibility]);

  const setColumnVisibility = useCallback(
    (updaterOrValue: MRT_Updater<MRT_VisibilityState>) => {
      let newVisibility: MRT_VisibilityState;
      if (typeof updaterOrValue === 'function') {
        newVisibility = updaterOrValue(columnVisibility);
      } else {
        newVisibility = updaterOrValue;
      }

      // Preserve internal MRT columns (like mrt-row-select) without pushing them to URL/LocalStorage
      const newInternalVisibility: MRT_VisibilityState = {};
      let internalChanged = false;
      MRT_INTERNAL_COLUMNS.forEach((id) => {
        if (id in newVisibility) {
          newInternalVisibility[id] = newVisibility[id];
          if (newInternalVisibility[id] !== internalVisibility[id]) {
            internalChanged = true;
          }
        }
      });

      if (internalChanged) {
        setInternalVisibility((prev) => ({
          ...prev,
          ...newInternalVisibility,
        }));
      }

      const newVisibleIds = sanitizeColumnIds(
        Object.keys(newVisibility),
      ).filter((id) => newVisibility[id]);
      setVisibleColumnIds(newVisibleIds);
    },
    [columnVisibility, setVisibleColumnIds, internalVisibility],
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
      rawColumns.map((c) => {
        return {
          id: getColumnId(c),
          label: typeof c.header === 'string' ? c.header : getColumnId(c),
        };
      }),
    [rawColumns],
  );

  const columns = useMemo(() => {
    const mappedColumns = rawColumns.map((colDef) => {
      const colId = getColumnId(colDef);
      if (!colId) return colDef;

      const isTemp = temporaryColumnIds.includes(colId);
      const isHighlighted = highlightedColumnIds.includes(colId);

      if (isTemp || isHighlighted) {
        return {
          ...colDef,
          enableHiding: isTemp ? false : colDef.enableHiding,
          meta: {
            ...(colDef.meta as Record<string, unknown> | undefined),
            isTemporary: isTemp,
            isHighlighted: isHighlighted,
          },
        };
      }
      return colDef;
    });

    if (!platform) return mappedColumns;

    return orderMRTColumns(
      platform,
      mappedColumns as MRT_ColumnDef<MRT_RowData>[],
      visibleColumnIds,
      temporaryColumnIds,
    );
  }, [
    rawColumns,
    temporaryColumnIds,
    highlightedColumnIds,
    visibleColumnIds,
    platform,
  ]) as TColumnDef[];

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
