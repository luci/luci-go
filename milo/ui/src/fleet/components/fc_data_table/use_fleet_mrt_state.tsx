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

import _ from 'lodash';
import {
  MRT_ColumnFiltersState,
  MRT_SortingState,
  MRT_RowSelectionState,
  MRT_ColumnSizingState,
} from 'material-react-table';
import { useCallback, useMemo, useState, useEffect, useRef } from 'react';

import {
  emptyPageTokenUpdater,
  nextPageTokenUpdater,
  pageSizeUpdater,
  prevPageTokenUpdater,
} from '@/common/components/params_pager';
import { PagerContext } from '@/common/components/params_pager/context';
import { logging } from '@/common/tools/logging';
import { filtersUpdater } from '@/fleet/components/filter_dropdown/search_param_utils';
import { StringListFilterCategory } from '@/fleet/components/filters/string_list_filter';
import { FilterCategory } from '@/fleet/components/filters/use_filters';
import { OptionCategory, StringListCategory } from '@/fleet/types/option';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { useMRTColumnManagement } from '../columns/use_mrt_column_management';
import { normalizeFilterKey } from '../filters/normalize_filter_key';

export type FleetColumnDefExt = {
  id?: string;
  accessorKey?: string | number | symbol;
  filterKey?: string;
  orderByField?: string;
  meta?: {
    isLoadingOptions?: boolean;
  };
};

export interface FleetMRTStateProps<
  TColumnDef extends FleetColumnDefExt = FleetColumnDefExt,
> {
  setSearchParams: (
    fn: (prev: URLSearchParams) => URLSearchParams,
    options?: { replace?: boolean },
  ) => void;
  pagerCtx: PagerContext;
  filterValues?: Record<string, FilterCategory>;

  /** Configuration for available filter options, typically mapped from backend Dimensions queries */
  filterOptionsConfig?: OptionCategory[];
  visibleColumns: TColumnDef[];
  localStorageKey: string;

  defaultColumnIds: string[];
  platform?: Platform;
  isLoadingOptions?: boolean;
  onColumnFiltersChangeOverride?: (
    updatedFilters: Record<string, string[]>,
  ) => void;
  orderByParam?: string;
}

export const useFleetMRTState = <
  TColumnDef extends FleetColumnDefExt = FleetColumnDefExt,
>({
  setSearchParams,
  pagerCtx,
  filterValues,
  filterOptionsConfig = [],
  visibleColumns,

  orderByParam,
  localStorageKey,
  defaultColumnIds,
  platform,
  onColumnFiltersChangeOverride,
  isLoadingOptions,
}: FleetMRTStateProps<TColumnDef>) => {
  const loadedKeyRef = useRef(localStorageKey);
  const [rowSelection, setRowSelection] = useState<MRT_RowSelectionState>({});

  const activeFilters = useMemo(() => {
    const filters: Record<string, string[]> = {};
    if (filterValues) {
      for (const [key, category] of Object.entries(filterValues)) {
        if (category.isActive()) {
          if (category instanceof StringListFilterCategory) {
            filters[key] = category.getSelectedOptions();
          }
        }
      }
    }
    return filters;
  }, [filterValues]);

  const [columnSizing, setColumnSizing] = useState<MRT_ColumnSizingState>(
    () => {
      try {
        const stored = localStorage.getItem(`${localStorageKey}-sizes`);
        const parsed = stored ? JSON.parse(stored) : null;
        return parsed || {};
      } catch (e) {
        logging.error('Failed to load column sizing from localStorage', e);
        return {};
      }
    },
  );

  // Sync columnSizing when localStorageKey changes
  useEffect(() => {
    if (loadedKeyRef.current !== localStorageKey) {
      try {
        const stored = localStorage.getItem(`${localStorageKey}-sizes`);
        const parsed = stored ? JSON.parse(stored) : null;
        setColumnSizing(parsed || {});
      } catch (e) {
        logging.error('Failed to load column sizing from localStorage', e);
        setColumnSizing({});
      }
      loadedKeyRef.current = localStorageKey;
    }
  }, [localStorageKey]);

  // Save columnSizing to localStorage when it changes (debounced to avoid UI jank during resizing)
  useEffect(() => {
    if (loadedKeyRef.current !== localStorageKey) {
      return;
    }

    const handler = setTimeout(() => {
      try {
        localStorage.setItem(
          `${localStorageKey}-sizes`,
          JSON.stringify(columnSizing),
        );
      } catch (e) {
        logging.error('Failed to save column sizing to localStorage', e);
      }
    }, 500);

    return () => {
      clearTimeout(handler);
    };
  }, [columnSizing, localStorageKey]);

  const resetColumnWidths = useCallback(() => {
    try {
      localStorage.removeItem(`${localStorageKey}-sizes`);
    } catch (e) {
      logging.error('Failed to remove column sizing from localStorage', e);
    }
    setColumnSizing({});
  }, [localStorageKey, setColumnSizing]);

  const { filterByFieldToId, idToFilterByField } = useMemo(() => {
    const fromFieldId = new Map<string, string>();
    const toFieldId = new Map<string, string>();
    visibleColumns.forEach((c) => {
      const cId = (c.id || c.accessorKey) as string;
      const filterKey = c.filterKey || cId;
      if (filterKey && cId) {
        fromFieldId.set(filterKey, cId);
        toFieldId.set(cId, filterKey);
      }
    });

    return { filterByFieldToId: fromFieldId, idToFilterByField: toFieldId };
  }, [visibleColumns]);

  const highlightedColumnIds = useMemo(() => {
    if (!filterValues) return [];

    return Object.entries(filterValues)
      .filter(([, category]) => category.isActive())
      .map(([key]) => {
        const filterKey = normalizeFilterKey(key);
        return filterByFieldToId.get(filterKey) || filterKey;
      });
  }, [filterValues, filterByFieldToId]);

  const mrtColumnManager = useMRTColumnManagement({
    localStorageKey,
    defaultColumnIds,
    columns: visibleColumns,
    highlightedColumnIds,
    platform,
  });

  const sorting: MRT_SortingState = useMemo(() => {
    if (!orderByParam) return [];
    return orderByParam.split(', ').map((sort: string) => {
      const match = mrtColumnManager.columns.find(
        (c) =>
          sort.startsWith(
            (c as FleetColumnDefExt)?.orderByField ?? (c.id as string),
          ) || sort.startsWith(c.id as string),
      );
      if (match) {
        return {
          id: (match.id || match.accessorKey || '') as string,
          desc: sort.endsWith(' desc'),
        };
      }
      return { id: sort.replace(' desc', ''), desc: sort.endsWith(' desc') };
    });
  }, [orderByParam, mrtColumnManager.columns]);

  // Optimization: Create a lookup map for filter options Using Normalized Keys
  const filterOptionsMap = useMemo(() => {
    const map = new Map<string, OptionCategory>();
    filterOptionsConfig.forEach((opt) => {
      if (opt.value) {
        const normalizedKey = normalizeFilterKey(String(opt.value));
        map.set(normalizedKey, opt);
      }
    });
    return map;
  }, [filterOptionsConfig]);

  const enrichedColumns = useMemo(() => {
    return mrtColumnManager.columns.map((col) => {
      const filterKey =
        (col as FleetColumnDefExt).filterKey || col.accessorKey || col.id;
      if (!filterKey) return col;

      const colWithMeta = {
        ...col,
        meta: {
          ...(col as FleetColumnDefExt).meta,
          isLoadingOptions,
        },
      };

      const normalizedFilterKey = normalizeFilterKey(String(filterKey));
      const option = filterOptionsMap.get(normalizedFilterKey);

      const isOptionCategory = (
        opt: OptionCategory,
      ): opt is StringListCategory => 'options' in opt;

      if (
        option &&
        isOptionCategory(option) &&
        option.options &&
        option.options.length > 0
      ) {
        return {
          ...colWithMeta,
          filterVariant: 'multi-select' as const,
          filterSelectOptions: option.options.map((opt) => ({
            text: opt.label,
            value: String(opt.value),
          })),
        };
      }
      return colWithMeta;
    });
  }, [mrtColumnManager.columns, filterOptionsMap, isLoadingOptions]);

  const columnFilters = useMemo(() => {
    return Object.entries(activeFilters).map(([id, value]) => {
      const colId = filterByFieldToId.get(normalizeFilterKey(id)) || id;
      return {
        id: colId,
        value,
      };
    });
  }, [activeFilters, filterByFieldToId]);

  // Reset row selection when filters change
  useEffect(() => {
    setRowSelection({});
  }, [columnFilters]);

  const onColumnFiltersChange = useCallback(
    (
      updater:
        | MRT_ColumnFiltersState
        | ((old: MRT_ColumnFiltersState) => MRT_ColumnFiltersState),
    ) => {
      const newFilters =
        typeof updater === 'function' ? updater(columnFilters) : updater;

      const newFilterOptions = newFilters.reduce(
        (
          acc: Record<string, string[]>,
          filter: { id: string; value: unknown },
        ) => {
          const urlKey = idToFilterByField.get(filter.id) || filter.id;
          acc[urlKey] =
            typeof filter.value === 'string'
              ? filter.value.split(',').map((v) => v.trim())
              : (filter.value as string[]);
          return acc;
        },
        {},
      );

      const isChanged = !_.isEqual(newFilterOptions, activeFilters);

      if (isChanged) {
        if (onColumnFiltersChangeOverride) {
          onColumnFiltersChangeOverride(newFilterOptions);
        } else {
          setSearchParams(filtersUpdater(newFilterOptions));
          setSearchParams((prev: URLSearchParams) =>
            emptyPageTokenUpdater(pagerCtx)(prev),
          );
        }
      }
    },
    [
      columnFilters,
      setSearchParams,
      idToFilterByField,
      activeFilters,
      pagerCtx,
      onColumnFiltersChangeOverride,
    ],
  );

  const onSortingChange = useCallback(
    (
      updater: MRT_SortingState | ((old: MRT_SortingState) => MRT_SortingState),
    ) => {
      const newSorting =
        typeof updater === 'function' ? updater(sorting) : updater;

      const orderByParamValue = newSorting
        .map((sort) => {
          const colDef = mrtColumnManager.columns.find(
            (c) => c.id === sort.id || c.accessorKey === sort.id,
          );
          const apiSortKey =
            (colDef as FleetColumnDefExt | undefined)?.orderByField ?? sort.id;
          return sort.desc ? `${apiSortKey} desc` : apiSortKey;
        })
        .join(', ');

      setSearchParams(
        (prev: URLSearchParams) => {
          const next = new URLSearchParams(prev);
          if (orderByParamValue) {
            next.set('order_by', orderByParamValue);
          } else {
            next.delete('order_by');
          }
          return emptyPageTokenUpdater(pagerCtx)(next);
        },
        { replace: true },
      );
    },
    [sorting, mrtColumnManager.columns, setSearchParams, pagerCtx],
  );

  const goToNextPage = useCallback(
    (nextPageToken: string) => {
      setSearchParams((prev: URLSearchParams) =>
        nextPageTokenUpdater(pagerCtx, nextPageToken)(prev),
      );
    },
    [pagerCtx, setSearchParams],
  );

  const goToPrevPage = useCallback(() => {
    setSearchParams((prev: URLSearchParams) =>
      prevPageTokenUpdater(pagerCtx)(prev),
    );
  }, [pagerCtx, setSearchParams]);

  const onRowsPerPageChange = useCallback(
    (pageSize: number) => {
      setSearchParams(pageSizeUpdater(pagerCtx, pageSize));
    },
    [pagerCtx, setSearchParams],
  );

  return useMemo(
    () => ({
      mrtColumnManager,
      sorting,
      enrichedColumns,
      columnFilters,
      onColumnFiltersChange,
      onSortingChange,
      goToNextPage,
      goToPrevPage,
      onRowsPerPageChange,
      visibleColumnIds: mrtColumnManager.visibleColumnIds,
      columnVisibility: mrtColumnManager.columnVisibility,
      allColumns: mrtColumnManager.allColumns,
      rowSelection,
      onRowSelectionChange: setRowSelection,
      columnSizing,
      onColumnSizingChange: setColumnSizing,
      resetColumnWidths,
    }),
    [
      mrtColumnManager,
      sorting,
      enrichedColumns,
      columnFilters,
      onColumnFiltersChange,
      onSortingChange,
      goToNextPage,
      goToPrevPage,
      onRowsPerPageChange,
      rowSelection,
      columnSizing,
      setColumnSizing,
      resetColumnWidths,
    ],
  );
};
