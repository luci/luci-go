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
} from 'material-react-table';
import { useCallback, useMemo, useState, useEffect } from 'react';

import {
  emptyPageTokenUpdater,
  nextPageTokenUpdater,
  pageSizeUpdater,
  prevPageTokenUpdater,
} from '@/common/components/params_pager';
import { PagerContext } from '@/common/components/params_pager/context';
import { GetFiltersResult } from '@/fleet/components/filter_dropdown/parser/parser';
import { filtersUpdater } from '@/fleet/components/filter_dropdown/search_param_utils';
import { OptionCategory, StringListCategory } from '@/fleet/types/option';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { useMRTColumnManagement } from '../columns/use_mrt_column_management';
import { normalizeFilterKey } from '../filters/normalize_filter_key';

export type FleetColumnDefExt = {
  id?: string;
  accessorKey?: string | number | symbol;
  filterByField?: string;
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
  selectedOptions: GetFiltersResult;

  /** Configuration for available filter options, typically mapped from backend Dimensions queries */
  filterOptionsConfig: OptionCategory[];
  columnsList: TColumnDef[];
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
  selectedOptions,
  filterOptionsConfig,
  columnsList,

  orderByParam,
  localStorageKey,
  defaultColumnIds,
  platform,
  onColumnFiltersChangeOverride,
  isLoadingOptions,
}: FleetMRTStateProps<TColumnDef>) => {
  const [rowSelection, setRowSelection] = useState<MRT_RowSelectionState>({});

  const { filterByFieldToId, idToFilterByField } = useMemo(() => {
    const fromFieldId = new Map<string, string>();
    const toFieldId = new Map<string, string>();
    columnsList.forEach((c) => {
      const cId = (c.id || c.accessorKey) as string;
      const filterKey = c.filterByField || cId;
      if (filterKey && cId) {
        fromFieldId.set(filterKey, cId);
        toFieldId.set(cId, filterKey);
      }
    });

    return { filterByFieldToId: fromFieldId, idToFilterByField: toFieldId };
  }, [columnsList]);

  const highlightedColumnIds = useMemo(() => {
    if (!selectedOptions?.filters) return [];

    return Object.keys(selectedOptions.filters).map((key) => {
      const filterKey = normalizeFilterKey(key);
      return filterByFieldToId.get(filterKey) || filterKey;
    });
  }, [selectedOptions?.filters, filterByFieldToId]);

  const mrtColumnManager = useMRTColumnManagement({
    localStorageKey,
    defaultColumnIds,
    columns: columnsList,
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
        (col as FleetColumnDefExt).filterByField || col.accessorKey || col.id;
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
    return Object.entries(selectedOptions?.filters || {}).map(([id, value]) => {
      const colId = filterByFieldToId.get(id) || id;
      return {
        id: colId,
        value,
      };
    });
  }, [selectedOptions?.filters, filterByFieldToId]);

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
          acc[urlKey] = filter.value as string[];
          return acc;
        },
        {},
      );

      const isChanged = !_.isEqual(
        newFilterOptions,
        selectedOptions?.filters || {},
      );

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
      selectedOptions?.filters,
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

  return {
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
  };
};
