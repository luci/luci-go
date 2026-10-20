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

import { androidState } from './android_state';

export const PREFERRED_STATUS_ORDER: string[] = [
  androidState.IDLE,
  androidState.BUSY,
  androidState.LAMEDUCK,
  androidState.MISSING,
  androidState.FAILED,
  androidState.DYING,
  androidState.INIT,
  androidState.DIRTY,
  androidState.PREPPING,
  'BLANK',
];

export function formatStateLabel(state: string): string {
  switch (state.toUpperCase()) {
    case androidState.IDLE:
      return 'Idle:';
    case androidState.BUSY:
      return 'Busy:';
    case androidState.LAMEDUCK:
      return 'Lameduck:';
    case androidState.MISSING:
      return 'Missing:';
    case androidState.FAILED:
      return 'Failed:';
    case androidState.DYING:
      return 'Dying:';
    case androidState.INIT:
      return 'Init:';
    case androidState.DIRTY:
      return 'Dirty:';
    case androidState.PREPPING:
      return 'Prepping:';
    case 'BLANK':
      return 'Blank states:';
    default:
      return state.charAt(0).toUpperCase() + state.slice(1).toLowerCase() + ':';
  }
}

export function getPlaceholderStatusKeys(
  showAllStates: boolean,
  alwaysShowBreakdown: boolean,
): string[] {
  if (!showAllStates && alwaysShowBreakdown) {
    return [androidState.IDLE, androidState.BUSY];
  }
  return PREFERRED_STATUS_ORDER;
}

export function getSortedStatusEntries(
  statusCounts: Record<string, number>,
  showAllStates: boolean,
  alwaysShowBreakdown: boolean,
): [string, number][] {
  let entries = Object.entries(statusCounts).sort(([a], [b]) => {
    const indexA = PREFERRED_STATUS_ORDER.indexOf(a.toUpperCase());
    const indexB = PREFERRED_STATUS_ORDER.indexOf(b.toUpperCase());
    if (indexA !== -1 && indexB !== -1) return indexA - indexB;
    if (indexA !== -1) return -1;
    if (indexB !== -1) return 1;
    return a.localeCompare(b);
  });

  if (!showAllStates && alwaysShowBreakdown) {
    entries = entries.filter(
      ([k]) =>
        k.toUpperCase() === androidState.IDLE ||
        k.toUpperCase() === androidState.BUSY,
    );
  }

  return entries;
}
