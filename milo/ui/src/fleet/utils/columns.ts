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

import { ANDROID_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

const COMMON_COLUMNS: Record<Platform, string[]> = {
  [Platform.UNSPECIFIED]: [],
  [Platform.ANDROID]: ANDROID_DEFAULT_COLUMNS,
  [Platform.CHROMEOS]: ['id', 'dut_id', 'state'],
  [Platform.CHROMIUM]: [
    'id',
    'ufs.hostname',
    'ufs.serial_number',
    'sw.state',
    'sw.dut_state',
    'sw.device_state',
  ],
};

const sortingComparator = (
  platform: Platform,
  a: string,
  b: string,
  visibleColumnIds: string[],
  temporaryColumnIds: string[],
) => {
  const aIsVisible = visibleColumnIds.includes(a) ? 1 : 0;
  const bIsVisible = visibleColumnIds.includes(b) ? 1 : 0;
  if (aIsVisible !== bIsVisible) {
    return bIsVisible - aIsVisible;
  }

  const aIsTemporary = temporaryColumnIds.includes(a) ? 1 : 0;
  const bIsTemporary = temporaryColumnIds.includes(b) ? 1 : 0;
  if (aIsTemporary !== bIsTemporary) {
    return bIsTemporary - aIsTemporary;
  }

  const aCommonIndex = COMMON_COLUMNS[platform].findIndex((c) => c === a);
  const bCommonIndex = COMMON_COLUMNS[platform].findIndex((c) => c === b);

  if (aCommonIndex !== -1 && bCommonIndex !== -1) {
    return aCommonIndex < bCommonIndex ? -1 : 1;
  }
  if (aCommonIndex !== -1 && bCommonIndex === -1) {
    return -1;
  }
  if (aCommonIndex === -1 && bCommonIndex !== -1) {
    return 1;
  }

  return a.localeCompare(b);
};

export const orderMRTColumns = <TData extends MRT_RowData>(
  platform: Platform,
  columnDefs: MRT_ColumnDef<TData>[],
  visibleColumnIds: string[] = [],
  temporaryColumnIds: string[] = [],
): MRT_ColumnDef<TData>[] => {
  return [...columnDefs].sort((a, b) => {
    const aId = (a.id || a.accessorKey) as string;
    const bId = (b.id || b.accessorKey) as string;
    if (!aId || !bId) return 0;
    return sortingComparator(
      platform,
      aId,
      bId,
      visibleColumnIds,
      temporaryColumnIds,
    );
  });
};
