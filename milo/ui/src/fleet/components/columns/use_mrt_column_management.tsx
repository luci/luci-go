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

export interface UseMRTColumnManagementProps<TData extends MRT_RowData> {
  columns: MRT_ColumnDef<TData>[];
  // IDs of columns that should be visible by default
  defaultColumnIds: string[];
  // Key for local storage persistence
  localStorageKey: string;
}

export function useMRTColumnManagement<TData extends MRT_RowData>({
  columns,
  defaultColumnIds,
  localStorageKey,
}: UseMRTColumnManagementProps<TData>) {
  const [visibleColumnIds, setVisibleColumnIds] = useParamsAndLocalStorage(
    COLUMNS_PARAM_KEY,
    localStorageKey,
    defaultColumnIds,
  );

  const columnVisibility = useMemo(() => {
    const visibility: MRT_VisibilityState = {};
    columns.forEach((col) => {
      const id = col.id || col.accessorKey?.toString();
      if (id) {
        visibility[id] = visibleColumnIds.includes(id);
      }
    });
    return visibility;
  }, [columns, visibleColumnIds]);

  const setColumnVisibility = useCallback(
    (updaterOrValue: MRT_Updater<MRT_VisibilityState>) => {
      let newVisibility: MRT_VisibilityState;
      if (typeof updaterOrValue === 'function') {
        newVisibility = updaterOrValue(columnVisibility);
      } else {
        newVisibility = updaterOrValue;
      }

      const newVisibleIds = Object.keys(newVisibility).filter(
        (id) => newVisibility[id],
      );
      setVisibleColumnIds(newVisibleIds);
    },
    [columnVisibility, setVisibleColumnIds],
  );

  const onToggleColumn = useCallback(
    (columnId: string) => {
      const isVisible = visibleColumnIds.includes(columnId);
      if (isVisible) {
        setVisibleColumnIds(visibleColumnIds.filter((id) => id !== columnId));
      } else {
        setVisibleColumnIds([...visibleColumnIds, columnId]);
      }
    },
    [visibleColumnIds, setVisibleColumnIds],
  );

  const allColumns = useMemo(
    () =>
      columns.map((c) => ({
        id: c.id || c.accessorKey?.toString() || '',
        label: c.header || c.id || c.accessorKey?.toString() || '',
      })),
    [columns],
  );

  return {
    columnVisibility,
    setColumnVisibility,
    visibleColumnIds,
    onToggleColumn,
    allColumns,
    resetDefaultColumns: () => setVisibleColumnIds([...defaultColumnIds]),
  };
}
