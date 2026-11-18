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
  getPinnedRepairRowProps,
  getRepairQueueRank,
  resolvePinnedRepairRows,
} from './pinned_rows';
import { UNKNOWN_USER_EMAIL } from './use_current_user_email';

const ME = 'operator1@example.com';

interface TestRow {
  readonly id: string;
  readonly claimedBy?: string;
}

const row = (id: string, claimedBy = ''): TestRow => ({ id, claimedBy });
const rowId = (item: TestRow) => item.id;

describe('pinned_rows', () => {
  it('computes only the leading contiguous caller-claimed prefix on initial/reordered snapshots', () => {
    const rows = [row('t1', ME), row('t2', ME), row('t3'), row('t4', ME)];
    expect([...resolvePinnedRepairRows(rows, ME, rowId).rowIndices]).toEqual([
      0, 1,
    ]);
    expect(
      resolvePinnedRepairRows(rows, UNKNOWN_USER_EMAIL, rowId).rowIndices.size,
    ).toBe(0);
    expect([
      ...resolvePinnedRepairRows([row('t1', ME)], ME, rowId).rowIndices,
    ]).toEqual([0]);
  });

  it('keeps subsequent pinned rows styled during in-place unclaims while ignoring mid-table claims', () => {
    const initial = resolvePinnedRepairRows(
      [row('t1', ME), row('t2', ME), row('t3')],
      ME,
      rowId,
    );
    expect([...initial.rowIndices]).toEqual([0, 1]);

    // Unclaiming t1 in place before refetch unpins row 0 while keeping row 1 pinned.
    const afterFirstUnclaim = resolvePinnedRepairRows(
      [row('t1'), row('t2', ME), row('t3')],
      ME,
      rowId,
      initial,
    );
    expect([...afterFirstUnclaim.rowIndices]).toEqual([1]);

    // Optimistically claiming mid-table t3 in place still does not mark t3 as pinned.
    const afterMidTableClaim = resolvePinnedRepairRows(
      [row('t1'), row('t2', ME), row('t3', ME)],
      ME,
      rowId,
      afterFirstUnclaim,
    );
    expect(afterMidTableClaim.rowIndices).toBe(afterFirstUnclaim.rowIndices);

    // Once the server refetch reorders the queue, the new leading prefix is pinned.
    const afterRefetch = resolvePinnedRepairRows(
      [row('t3', ME), row('t2', ME), row('t1')],
      ME,
      rowId,
      afterMidTableClaim,
    );
    expect([...afterRefetch.rowIndices]).toEqual([0, 1]);

    // Unclaiming all rows prunes pinnedIds so re-claiming row 1 below row 0 stays unpinned.
    const afterUnclaimAll = resolvePinnedRepairRows(
      [row('t3'), row('t2'), row('t1')],
      ME,
      rowId,
      afterRefetch,
    );
    expect(afterUnclaimAll.rowIndices.size).toBe(0);
    expect(
      resolvePinnedRepairRows(
        [row('t3'), row('t2', ME), row('t1')],
        ME,
        rowId,
        afterUnclaimAll,
      ).rowIndices.size,
    ).toBe(0);
  });

  it('prefers backend objective rank and falls back to 1-based page offset', () => {
    expect(getRepairQueueRank(42, 0, 100, 0)).toBe(42);
    expect(getRepairQueueRank(0, 1, 50, 2)).toBe(53);
    expect(getRepairQueueRank(undefined, 0, 100, 3)).toBe(4);
  });

  it('marks only pinned rows and adds divider only when unpinned rows follow', () => {
    const twoPinned = new Set([0, 1]);
    expect(getPinnedRepairRowProps(2, twoPinned, 4)).toEqual({});
    expect(getPinnedRepairRowProps(0, twoPinned, 4)['data-pinned']).toBe(
      'true',
    );
    expect(getPinnedRepairRowProps(1, twoPinned, 4)['data-pinned']).toBe(
      'true',
    );
    expect(getPinnedRepairRowProps(1, twoPinned, 2)['data-pinned']).toBe(
      'true',
    );

    const secondOnlyPinned = new Set([1]);
    expect(getPinnedRepairRowProps(0, secondOnlyPinned, 3)).toEqual({});
    expect(getPinnedRepairRowProps(1, secondOnlyPinned, 3)['data-pinned']).toBe(
      'true',
    );
  });
});
