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

import { TableRowProps } from '@mui/material';
import { useRef } from 'react';

import { fleetColors } from '@/fleet/theme/tokens';

import { UNKNOWN_USER_EMAIL } from './use_current_user_email';

const EMPTY_ROW_INDICES: ReadonlySet<number> = new Set<number>();

export interface PinnedRowsSnapshot {
  readonly currentUser: string;
  readonly rowIds: readonly string[];
  readonly rowIndices: ReadonlySet<number>;
}

/**
 * Resolves the 0-based row indices in `items` that should be styled as pinned
 * at the top of the queue (`EMPTY_ROW_INDICES` when the user is unknown).
 *
 * Scans the contiguous leading prefix claimed by `currentUser` so mid-table
 * optimistic claims are not styled as "Pinned to top" before the server
 * reorders the queue, while retaining previously pinned indices when row order
 * is unchanged so unclaiming an earlier pinned row in place does not strip
 * pinned styling from remaining pinned rows before refetch.
 */
export const resolvePinnedRepairRows = <
  T extends { readonly claimedBy?: string },
>(
  items: readonly T[],
  currentUser: string,
  getRowId: (item: T) => string,
  prev: PinnedRowsSnapshot | null = null,
): PinnedRowsSnapshot => {
  if (!currentUser || currentUser === UNKNOWN_USER_EMAIL || !items.length) {
    return { currentUser, rowIds: [], rowIndices: EMPTY_ROW_INDICES };
  }

  const rowIds = items.map(getRowId);
  const isSameRowOrder =
    prev !== null &&
    prev.rowIndices.size > 0 &&
    prev.currentUser === currentUser &&
    prev.rowIds.length === rowIds.length &&
    prev.rowIds.every((id, idx) => id === rowIds[idx]);

  const rowIndices = new Set<number>();
  let inLeadingPrefix = true;

  for (let i = 0; i < items.length; i++) {
    if (items[i].claimedBy !== currentUser) {
      if (!isSameRowOrder) break;
      inLeadingPrefix = false;
      continue;
    }
    if (inLeadingPrefix || (isSameRowOrder && prev.rowIndices.has(i))) {
      rowIndices.add(i);
    }
  }

  if (!rowIndices.size) {
    return { currentUser, rowIds: [], rowIndices: EMPTY_ROW_INDICES };
  }

  const stableRowIndices =
    prev &&
    prev.rowIndices.size === rowIndices.size &&
    [...rowIndices].every((idx) => prev.rowIndices.has(idx))
      ? prev.rowIndices
      : rowIndices;

  return { currentUser, rowIds, rowIndices: stableRowIndices };
};

/**
 * Tracks pinned row indices across renders so in-place optimistic unclaims do
 * not strip pinned styling from subsequent pinned rows while awaiting refetch.
 *
 * `resolvePinnedRepairRows` is idempotent (`f(x, f(x, s)) === f(x, s)`), making
 * the render-time ref cache safe under StrictMode double-renders without
 * triggering extra render passes when callers pass fresh array literals.
 */
export const usePinnedRepairRows = <T extends { readonly claimedBy?: string }>(
  items: readonly T[],
  currentUser: string,
  getRowId: (item: T) => string,
): ReadonlySet<number> => {
  const cachedRef = useRef<{
    readonly items: readonly T[];
    readonly snapshot: PinnedRowsSnapshot;
  } | null>(null);
  const prev = cachedRef.current;

  if (
    prev &&
    prev.items === items &&
    prev.snapshot.currentUser === currentUser
  ) {
    return prev.snapshot.rowIndices;
  }

  const snapshot = resolvePinnedRepairRows(
    items,
    currentUser,
    getRowId,
    prev?.snapshot ?? null,
  );
  cachedRef.current = { items, snapshot };
  return snapshot.rowIndices;
};

/**
 * Resolves the 1-based objective queue rank of a row, falling back to its
 * page-offset position when `itemRank` is unset (0 or undefined).
 */
export const getRepairQueueRank = (
  itemRank: number | undefined,
  pageIndex: number,
  pageSize: number,
  rowIndex: number,
): number => itemRank || pageIndex * pageSize + rowIndex + 1;

/**
 * Builds Material React Table `muiTableBodyRowProps` for a pinned caller-claimed
 * repair row (tonal tint, hover highlight, and a bottom divider on the last
 * pinned row when unpinned rows follow).
 */
export const getPinnedRepairRowProps = (
  rowIndex: number,
  pinnedRowIndices: ReadonlySet<number>,
  totalRows: number,
): TableRowProps & { 'data-pinned'?: string } => {
  if (!pinnedRowIndices.has(rowIndex)) {
    return {};
  }
  const lastPinned = Math.max(...pinnedRowIndices);
  const isLastPinned = rowIndex === lastPinned && lastPinned < totalRows - 1;
  return {
    'data-pinned': 'true',
    sx: {
      '& > .MuiTableCell-root': {
        backgroundColor: fleetColors.surfaceTonal,
        ...(isLastPinned && {
          borderBottom: `2px solid ${fleetColors.borderPrimary}`,
        }),
      },
      '&:not(.Mui-selected):hover > .MuiTableCell-root': {
        backgroundColor: fleetColors.primaryContainer,
      },
    },
  };
};
