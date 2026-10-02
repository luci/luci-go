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

import { MRT_ColumnDef, MRT_RowData } from 'material-react-table';
import { useMemo } from 'react';

import { getColumnId } from '@/fleet/components/columns/use_mrt_column_management';

import { useMrtSortingState } from './use_mrt_sorting_state';

/**
 * Custom hook that combines URL-based sorting state management with client-side
 * data sorting for Material React Table.
 *
 * This hook resolves sort columns using either `id` or `accessorKey` and supports
 * custom `sortingFn` when provided on the column definition.
 */
export function useMrtSorting<TData extends MRT_RowData>(
  rows: TData[],
  columns: MRT_ColumnDef<TData>[],
) {
  const [sorting, onSortingChange] = useMrtSortingState(columns);

  const sortedRows = useMemo(() => {
    if (!sorting || sorting.length === 0) return rows;

    return [...rows].sort((a, b) => {
      for (const sort of sorting) {
        const col = columns.find((c) => getColumnId(c) === sort.id);
        if (!col) continue;

        let cmp = 0;
        if (typeof col.sortingFn === 'function') {
          const resolveCellValue = (row: TData, id: string) => {
            const targetCol =
              id === sort.id
                ? col
                : (columns.find((c) => getColumnId(c) === id) ?? col);
            return targetCol.accessorFn
              ? targetCol.accessorFn(row)
              : row[(targetCol.accessorKey as string) ?? id];
          };
          const mockRowA = {
            original: a,
            getValue: (id: string) => resolveCellValue(a, id),
          } as Parameters<typeof col.sortingFn>[0];
          const mockRowB = {
            original: b,
            getValue: (id: string) => resolveCellValue(b, id),
          } as Parameters<typeof col.sortingFn>[1];
          cmp = col.sortingFn(mockRowA, mockRowB, sort.id);
        } else {
          const valA = col.accessorFn
            ? col.accessorFn(a)
            : a[(col.accessorKey as string) ?? sort.id];
          const valB = col.accessorFn
            ? col.accessorFn(b)
            : b[(col.accessorKey as string) ?? sort.id];

          const strA = String(valA ?? '');
          const strB = String(valB ?? '');
          cmp = strA.localeCompare(strB, undefined, { numeric: true });
        }

        if (cmp !== 0) {
          return sort.desc ? -cmp : cmp;
        }
      }
      return 0;
    });
  }, [rows, sorting, columns]);

  return {
    sorting,
    onSortingChange,
    sortedRows,
  };
}
