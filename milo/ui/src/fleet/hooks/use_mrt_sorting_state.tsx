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
  MRT_RowData,
  MRT_SortingState,
  MRT_Updater,
} from 'material-react-table';
import { useCallback, useMemo } from 'react';

import {
  emptyPageTokenUpdater,
  PagerContext,
} from '@/common/components/params_pager';
import { getColumnId } from '@/fleet/components/columns/use_mrt_column_management';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { ORDER_BY_PARAM_KEY } from './order_by';

/**
 * Hook for managing the Material React Table (MRT) sorting state.
 * Syncs the sorting state with the `order_by` search parameter. If a `pagerCtx` is provided,
 * changing the sorting direction or sorted column will atomically reset the pagination token.
 */
export function useMrtSortingState<TData extends MRT_RowData = MRT_RowData>(
  columns?:
    | MRT_ColumnDef<TData>[]
    | Array<{ id?: string; orderByField?: string }>,
  pagerCtx?: PagerContext,
): [
  MRT_SortingState,
  (updater: MRT_Updater<MRT_SortingState>) => void,
  string,
] {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const orderByParam = searchParams.get(ORDER_BY_PARAM_KEY) ?? '';

  const sorting = useMemo<MRT_SortingState>(() => {
    if (!orderByParam) return [];
    const rawItems = orderByParam
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    return rawItems
      .map((item) => {
        const parts = item.split(/\s+/);
        const id = parts[0];
        const desc = parts[1]?.toLowerCase() === 'desc';

        if (!id) return null;

        if (!columns || columns.length === 0) {
          return { id, desc };
        }

        const col = columns.find(
          (c) =>
            getColumnId(c as MRT_ColumnDef<TData>) === id ||
            (c as { orderByField?: string }).orderByField === id,
        );

        const sortId = (col && getColumnId(col as MRT_ColumnDef<TData>)) || id;

        return { id: sortId, desc };
      })
      .filter((s): s is { id: string; desc: boolean } => Boolean(s && s.id));
  }, [orderByParam, columns]);

  const onSortingChange = useCallback(
    (updater: MRT_Updater<MRT_SortingState>) => {
      const newSorting =
        typeof updater === 'function' ? updater(sorting) : updater;

      const nextOrderBy = newSorting
        .map((s) => {
          const col = columns?.find(
            (c) => getColumnId(c as MRT_ColumnDef<TData>) === s.id,
          );
          const field =
            (col as { orderByField?: string })?.orderByField ?? s.id;
          return s.desc ? `${field} desc` : field;
        })
        .join(', ');

      if (orderByParam !== nextOrderBy) {
        setSearchParams((prev) => {
          const next = new URLSearchParams(prev);
          if (nextOrderBy === '') {
            next.delete(ORDER_BY_PARAM_KEY);
          } else {
            next.set(ORDER_BY_PARAM_KEY, nextOrderBy);
          }
          if (pagerCtx) {
            return emptyPageTokenUpdater(pagerCtx)(next);
          }
          return next;
        });
      }
    },
    [sorting, columns, orderByParam, pagerCtx, setSearchParams],
  );

  return [sorting, onSortingChange, orderByParam];
}
