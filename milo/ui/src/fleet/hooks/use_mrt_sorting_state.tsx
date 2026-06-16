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

import { MRT_SortingState, MRT_Updater } from 'material-react-table';
import { useCallback } from 'react';

import {
  emptyPageTokenUpdater,
  PagerContext,
} from '@/common/components/params_pager';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { ORDER_BY_PARAM_KEY } from './order_by';
import { useMrtSorting } from './use_mrt_sorting';

/**
 * Hook for managing the Material React Table (MRT) sorting state.
 * Syncs the sorting state with the `order_by` search parameter. If a `pagerCtx` is provided,
 * changing the sorting direction or sorted column will atomically reset the pagination token.
 */
export function useMrtSortingState(
  columns?: Array<{ id: string; orderByField?: string }>,
  pagerCtx?: PagerContext,
): [
  MRT_SortingState,
  (updater: MRT_Updater<MRT_SortingState>) => void,
  string,
] {
  const sorting = useMrtSorting(columns);
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const orderByParam = searchParams.get(ORDER_BY_PARAM_KEY) ?? '';

  const onSortingChange = useCallback(
    (updater: MRT_Updater<MRT_SortingState>) => {
      const newSorting =
        typeof updater === 'function' ? updater(sorting) : updater;

      const nextOrderBy = newSorting
        .map((s) => {
          const col = columns?.find((c) => c.id === s.id);
          const field = col?.orderByField ?? s.id;
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
