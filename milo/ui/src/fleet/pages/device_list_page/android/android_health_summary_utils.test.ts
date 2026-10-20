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
  formatStateLabel,
  getPlaceholderStatusKeys,
  getSortedStatusEntries,
  PREFERRED_STATUS_ORDER,
} from './android_health_summary_utils';
import { androidState } from './android_state';

describe('android_health_summary_utils', () => {
  describe('formatStateLabel', () => {
    it('formats known android states and BLANK correctly', () => {
      expect(formatStateLabel(androidState.IDLE)).toBe('Idle:');
      expect(formatStateLabel(androidState.BUSY)).toBe('Busy:');
      expect(formatStateLabel(androidState.LAMEDUCK)).toBe('Lameduck:');
      expect(formatStateLabel(androidState.MISSING)).toBe('Missing:');
      expect(formatStateLabel(androidState.FAILED)).toBe('Failed:');
      expect(formatStateLabel(androidState.DYING)).toBe('Dying:');
      expect(formatStateLabel(androidState.INIT)).toBe('Init:');
      expect(formatStateLabel(androidState.DIRTY)).toBe('Dirty:');
      expect(formatStateLabel(androidState.PREPPING)).toBe('Prepping:');
      expect(formatStateLabel('BLANK')).toBe('Blank states:');
    });

    it('capitalizes unknown state keys', () => {
      expect(formatStateLabel('custom_state')).toBe('Custom_state:');
    });
  });

  describe('getPlaceholderStatusKeys', () => {
    it('returns IDLE and BUSY when showAllStates is false and alwaysShowBreakdown is true', () => {
      expect(getPlaceholderStatusKeys(false, true)).toEqual([
        androidState.IDLE,
        androidState.BUSY,
      ]);
    });

    it('returns PREFERRED_STATUS_ORDER when showAllStates is true', () => {
      expect(getPlaceholderStatusKeys(true, false)).toEqual(
        PREFERRED_STATUS_ORDER,
      );
      expect(getPlaceholderStatusKeys(true, true)).toEqual(
        PREFERRED_STATUS_ORDER,
      );
    });
  });

  describe('getSortedStatusEntries', () => {
    it('sorts according to PREFERRED_STATUS_ORDER with fallback to alphabetical', () => {
      const counts: Record<string, number> = {
        BUSY: 5,
        IDLE: 10,
        UNKNOWN_B: 2,
        UNKNOWN_A: 1,
        FAILED: 3,
      };

      const result = getSortedStatusEntries(counts, true, false);
      expect(result).toEqual([
        ['IDLE', 10],
        ['BUSY', 5],
        ['FAILED', 3],
        ['UNKNOWN_A', 1],
        ['UNKNOWN_B', 2],
      ]);
    });

    it('filters down to IDLE and BUSY when showAllStates is false and alwaysShowBreakdown is true', () => {
      const counts: Record<string, number> = {
        BUSY: 5,
        IDLE: 10,
        FAILED: 3,
      };

      const result = getSortedStatusEntries(counts, false, true);
      expect(result).toEqual([
        ['IDLE', 10],
        ['BUSY', 5],
      ]);
    });
  });
});
