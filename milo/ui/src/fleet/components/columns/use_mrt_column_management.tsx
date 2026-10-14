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

/* eslint-disable react-refresh/only-export-components */

import {
  MRT_ColumnDef,
  MRT_VisibilityState,
  MRT_Updater,
  MRT_RowData,
} from 'material-react-table';
import { useMemo, useCallback, useState, useRef, useEffect } from 'react';

import { normalizeFilterKey } from '@/fleet/components/filters/normalize_filter_key';
import { FilterCategory } from '@/fleet/components/filters/use_filters';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { orderMRTColumns } from '@/fleet/utils/columns';
import { humanizeColumnLabel } from '@/fleet/utils/humanize_column';
import { useWarnings } from '@/fleet/utils/use_warnings';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { useParamsAndLocalStorage } from './use_params_and_local_storage';

export interface ColumnOption {
  id: string;
  label: string;
  isTemporary?: boolean;
  isHighlighted?: boolean;
}

export interface MrtColumnManager<TColumnDef> {
  columns: TColumnDef[];
  columnVisibility: MRT_VisibilityState;
  setColumnVisibility: (
    updater:
      | MRT_VisibilityState
      | ((old: MRT_VisibilityState) => MRT_VisibilityState),
  ) => void;
  visibleColumnIds: string[];
  setVisibleColumnIds: (
    update: string[] | ((prev: string[]) => string[]),
  ) => void;
  onToggleColumn: (id: string) => void;
  selectOnlyColumn: (id: string) => void;
  allColumns: ColumnOption[];
  resetDefaultColumns: () => void;
  warnings: string[];
}

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
    filterKey?: string;
  } = MRT_ColumnDef<_TData>,
> {
  columns: TColumnDef[];
  // IDs of columns that should be visible by default
  defaultColumnIds: string[];
  // Key for local storage persistence
  localStorageKey: string;
  // Active filter values to automatically highlight columns and sync warnings
  filterValues?: Record<string, FilterCategory> | undefined;
  isLoadingColumns?: boolean;
  isLoadingFilters?: boolean;
  isLoadingDevices?: boolean;
  // IDs of columns that are currently filtered and should be highlighted (manual override)
  highlightedColumnIds?: readonly string[];
  platform?: Platform;
}

const EMPTY_ARRAY: readonly string[] = [];

export function useMRTColumnManagement<
  _TData extends MRT_RowData,
  TColumnDef extends {
    id?: string;
    accessorKey?: string | number | symbol;
    header?: unknown;
    meta?: unknown;
    enableHiding?: boolean;
    filterKey?: string;
  } = MRT_ColumnDef<_TData>,
>({
  columns: rawColumns,
  defaultColumnIds,
  localStorageKey,
  filterValues,
  isLoadingColumns = false,
  isLoadingFilters = false,
  isLoadingDevices = false,
  highlightedColumnIds: manualHighlightedColumnIds,
  platform,
}: UseMRTColumnManagementProps<_TData, TColumnDef>) {
  const defaultColumnIdsRef = useRef(defaultColumnIds);
  useEffect(() => {
    defaultColumnIdsRef.current = defaultColumnIds;
  }, [defaultColumnIds]);

  const [visibleColumnIds, setVisibleColumnIds] = useParamsAndLocalStorage(
    COLUMNS_PARAM_KEY,
    localStorageKey,
    defaultColumnIds,
    sanitizeColumnIds,
  );

  const filterByFieldToId = useMemo(() => {
    const fromFieldId = new Map<string, string>();
    rawColumns.forEach((c) => {
      const cId = getColumnId(c);
      const filterKey = c.filterKey || cId;
      if (filterKey && cId) {
        fromFieldId.set(normalizeFilterKey(filterKey), cId);
        fromFieldId.set(filterKey, cId);
      }
    });
    return fromFieldId;
  }, [rawColumns]);

  const highlightedColumnIds = useMemo(() => {
    if (manualHighlightedColumnIds) return manualHighlightedColumnIds;
    if (!filterValues) return EMPTY_ARRAY;
    const mapped = Object.entries(filterValues)
      .filter(([, cat]) => cat.isActive())
      .map(([key]) => {
        const norm = normalizeFilterKey(key);
        return (
          filterByFieldToId.get(norm) || filterByFieldToId.get(key) || norm
        );
      });
    return Array.from(new Set(mapped));
  }, [filterValues, filterByFieldToId, manualHighlightedColumnIds]);

  // Maps actively filtered column IDs that the user explicitly hid during the
  // current filter session to their pre-dismissal origin ('permanent' | 'temporary').
  // Permanent columns must also be recorded here when hidden while filtered so
  // `temporaryColumnIds` does not immediately re-enable them, while preserving
  // whether re-enabling them should restore them to `visibleColumnIds`.
  const [hiddenFilteredColumns, setHiddenFilteredColumns] = useState<
    Record<string, 'permanent' | 'temporary'>
  >({});

  useEffect(() => {
    if (isLoadingColumns || isLoadingFilters) return;
    setHiddenFilteredColumns((prev) => {
      const nextEntries = Object.entries(prev).filter(
        ([id]) =>
          highlightedColumnIds.includes(id) && !visibleColumnIds.includes(id),
      );
      return nextEntries.length === Object.keys(prev).length
        ? prev
        : Object.fromEntries(nextEntries);
    });
  }, [
    highlightedColumnIds,
    visibleColumnIds,
    isLoadingColumns,
    isLoadingFilters,
  ]);

  const temporaryColumnIds = useMemo(() => {
    if (highlightedColumnIds.length === 0) return EMPTY_ARRAY;
    return highlightedColumnIds.filter(
      (col) =>
        !visibleColumnIds.includes(col) && !(col in hiddenFilteredColumns),
    );
  }, [highlightedColumnIds, visibleColumnIds, hiddenFilteredColumns]);

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

      if (highlightedColumnIds.length > 0) {
        setHiddenFilteredColumns((prev) => {
          const next = { ...prev };
          highlightedColumnIds.forEach((id) => {
            if (newVisibility[id] === true) {
              delete next[id];
            } else if (newVisibility[id] === false && !(id in next)) {
              next[id] = visibleColumnIds.includes(id)
                ? 'permanent'
                : 'temporary';
            }
          });
          return next;
        });
      }

      const newVisibleIds = sanitizeColumnIds(
        Object.keys(newVisibility),
      ).filter(
        (id) =>
          Boolean(newVisibility[id]) &&
          (visibleColumnIds.includes(id) ||
            hiddenFilteredColumns[id] === 'permanent' ||
            !highlightedColumnIds.includes(id)),
      );
      setVisibleColumnIds(newVisibleIds);
    },
    [
      columnVisibility,
      setVisibleColumnIds,
      internalVisibility,
      highlightedColumnIds,
      visibleColumnIds,
      hiddenFilteredColumns,
    ],
  );

  const onToggleColumn = useCallback(
    (columnId: string) => {
      const isVisible =
        visibleColumnIds.includes(columnId) ||
        temporaryColumnIds.includes(columnId);
      if (isVisible) {
        if (highlightedColumnIds.includes(columnId)) {
          const origin = visibleColumnIds.includes(columnId)
            ? 'permanent'
            : 'temporary';
          setHiddenFilteredColumns((prev) =>
            prev[columnId] === origin ? prev : { ...prev, [columnId]: origin },
          );
        }
        if (visibleColumnIds.includes(columnId)) {
          setVisibleColumnIds(visibleColumnIds.filter((id) => id !== columnId));
        }
      } else {
        const wasDismissedTemp =
          highlightedColumnIds.includes(columnId) &&
          hiddenFilteredColumns[columnId] === 'temporary';
        setHiddenFilteredColumns((prev) => {
          if (!(columnId in prev)) return prev;
          const next = { ...prev };
          delete next[columnId];
          return next;
        });
        if (!wasDismissedTemp) {
          setVisibleColumnIds([...visibleColumnIds, columnId]);
        }
      }
    },
    [
      visibleColumnIds,
      setVisibleColumnIds,
      temporaryColumnIds,
      highlightedColumnIds,
      hiddenFilteredColumns,
    ],
  );

  const selectOnlyColumn = useCallback(
    (columnId: string) => {
      const nextHidden: Record<string, 'permanent' | 'temporary'> = {};
      for (const id of highlightedColumnIds) {
        if (id !== columnId) {
          nextHidden[id] = visibleColumnIds.includes(id)
            ? 'permanent'
            : 'temporary';
        }
      }
      setHiddenFilteredColumns(nextHidden);
      setVisibleColumnIds([columnId]);
    },
    [highlightedColumnIds, visibleColumnIds, setVisibleColumnIds],
  );

  const allColumns = useMemo(
    () =>
      rawColumns.map((c) => {
        const id = getColumnId(c);
        const isTemporary = temporaryColumnIds.includes(id);
        const isHighlighted = highlightedColumnIds.includes(id);
        return {
          id,
          label:
            typeof c.header === 'string' && c.header
              ? c.header
              : humanizeColumnLabel(id),
          ...(isTemporary ? { isTemporary: true } : {}),
          ...(isHighlighted ? { isHighlighted: true } : {}),
        };
      }),
    [rawColumns, temporaryColumnIds, highlightedColumnIds],
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

  const resetDefaultColumns = useCallback(() => {
    setHiddenFilteredColumns({});
    setVisibleColumnIds([...defaultColumnIdsRef.current]);
  }, [setVisibleColumnIds]);

  const [warnings, addWarning] = useWarnings();

  useEffect(() => {
    if (isLoadingColumns || isLoadingFilters || isLoadingDevices) return;

    const invalidCols = visibleColumnIds.filter(
      (id) => !rawColumns.some((col) => getColumnId(col) === id),
    );

    if (invalidCols.length === 0) return;

    addWarning(
      `The following columns are not available: ${invalidCols.join(', ')}`,
    );

    setVisibleColumnIds((prev) =>
      prev.filter((id) => !invalidCols.includes(id)),
    );
  }, [
    addWarning,
    rawColumns,
    visibleColumnIds,
    isLoadingColumns,
    isLoadingFilters,
    isLoadingDevices,
    setVisibleColumnIds,
  ]);

  return useMemo(
    () => ({
      columns,
      columnVisibility,
      setColumnVisibility,
      visibleColumnIds,
      setVisibleColumnIds,
      onToggleColumn,
      selectOnlyColumn,
      allColumns,
      resetDefaultColumns,
      warnings,
    }),
    [
      columns,
      columnVisibility,
      setColumnVisibility,
      visibleColumnIds,
      setVisibleColumnIds,
      onToggleColumn,
      selectOnlyColumn,
      allColumns,
      resetDefaultColumns,
      warnings,
    ],
  );
}
