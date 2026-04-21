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

import { MRT_RowData, MRT_TableInstance } from 'material-react-table';

import { PagerContext } from '@/common/components/params_pager';

export const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100, 500, 1000];

export interface FleetTableMeta<TData extends MRT_RowData> {
  totalSize?: number;
  nextPageToken?: string;
  goToPrevPage: () => void;
  goToNextPage: (token: string) => void;
  onRowsPerPageChange: (size: number) => void;
  pagerCtx: PagerContext;
  searchParams: URLSearchParams;

  // For TopToolbar (optional)
  allDimensionColumns?: { id: string; label: string }[];
  visibleColumnIds?: string[];
  onToggleColumn?: (id: string) => void;
  selectOnlyColumn?: (id: string) => void;
  resetDefaultColumns?: () => void;
  handleCopy?: (table: MRT_TableInstance<TData>) => void;
}

export function useFleetTableMeta<TData extends MRT_RowData>(
  table: MRT_TableInstance<TData>,
): FleetTableMeta<TData> {
  const meta = table.options.meta as FleetTableMeta<TData>;
  if (!meta) {
    throw new Error('FleetTableMeta is missing in table options!');
  }
  return meta;
}
