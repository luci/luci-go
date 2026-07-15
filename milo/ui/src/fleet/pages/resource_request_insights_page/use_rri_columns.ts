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

import { useMemo } from 'react';

import {
  getColumnId,
  useMRTColumnManagement,
} from '@/fleet/components/columns/use_mrt_column_management';
import { FilterCategory } from '@/fleet/components/filters/use_filters';
import { RRI_DEVICES_COLUMNS_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';

import { COLUMNS, DEFAULT_COLUMNS } from './rri_columns';

export const useRriColumns = (
  filterValues: Record<string, FilterCategory> | undefined,
  isLoadingFilters: boolean,
  isLoadingDevices: boolean,
) => {
  const availableColumns = useMemo(() => Object.values(COLUMNS), []);

  const defaultColumnIds = useMemo(
    () =>
      availableColumns
        .filter((c) => DEFAULT_COLUMNS.includes(getColumnId(c)))
        .map((c) => getColumnId(c)),
    [availableColumns],
  );

  const mrtColumnManager = useMRTColumnManagement({
    columns: availableColumns,
    defaultColumnIds,
    localStorageKey: RRI_DEVICES_COLUMNS_LOCAL_STORAGE_KEY,
    filterValues,
    isLoadingFilters,
    isLoadingDevices,
  });

  return {
    mrtColumnManager,
    warnings: mrtColumnManager.warnings,
  };
};
