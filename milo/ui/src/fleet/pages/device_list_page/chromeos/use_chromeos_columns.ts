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

import { useMemo, useEffect } from 'react';

import { useMRTColumnManagement } from '@/fleet/components/columns/use_mrt_column_management';
import { normalizeFilterKey } from '@/fleet/components/filters/normalize_filter_key';
import { FilterCategory } from '@/fleet/components/filters/use_filters';
import { CHROMEOS_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { CHROMEOS_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useWarnings } from '@/fleet/utils/use_warnings';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';

import { getFieldDefinition } from './chromeos_fields';
import { useChromeOSFields } from './use_chromeos_available_columns';

export const useChromeOSColumns = (
  filterValues: Record<string, FilterCategory> | undefined,
  isLoadingFilters: boolean,
  isLoadingDevices: boolean,
) => {
  const { availableFields } = useChromeOSFields();
  const [searchParams] = useSyncedSearchParams();

  const urlCols = useMemo(() => {
    const columnsParamStr = searchParams.getAll(COLUMNS_PARAM_KEY).join(',');
    return columnsParamStr ? columnsParamStr.split(',') : [];
  }, [searchParams]);

  const availableColumns = useMemo(() => {
    const baseCols = availableFields.map((def) => def.columnDef);
    const baseColIds = new Set(baseCols.map((col) => col.id));

    const missingCols = urlCols.flatMap((id) => {
      if (baseColIds.has(id)) return [];
      const def = getFieldDefinition(id);
      return def?.columnDef ? [def.columnDef] : [];
    });

    return [...baseCols, ...missingCols];
  }, [availableFields, urlCols]);

  const { filterByFieldToId } = useMemo(() => {
    const fromFieldId = new Map<string, string>();
    availableColumns.forEach((c) => {
      const cId = c.id;
      const filterKey = c.filterKey || cId;
      if (filterKey && cId) {
        fromFieldId.set(filterKey, cId);
      }
    });
    return { filterByFieldToId: fromFieldId };
  }, [availableColumns]);

  const highlightedColumnIds = useMemo(() => {
    if (!filterValues) return [];
    return Object.entries(filterValues)
      .filter(([, cat]) => cat.isActive())
      .map(([key]) => {
        const norm = normalizeFilterKey(key);
        return filterByFieldToId.get(norm) || norm;
      });
  }, [filterValues, filterByFieldToId]);

  const mrtColumnManager = useMRTColumnManagement({
    columns: availableColumns,
    defaultColumnIds: CHROMEOS_DEFAULT_COLUMNS,
    localStorageKey: CHROMEOS_DEVICES_LOCAL_STORAGE_KEY,
    highlightedColumnIds,
    platform: Platform.CHROMEOS,
  });

  const [warnings, addWarning] = useWarnings();

  // Validates the column search parameters and alerts the user of any invalid columns.
  // This is purely for validation and UI alert purposes. Syncing of columns between
  // the URL and local state is handled entirely inside useMRTColumnManagement.
  useEffect(() => {
    if (isLoadingFilters || isLoadingDevices) return;

    const invalidCols = mrtColumnManager.visibleColumnIds.filter(
      (id) => !availableColumns.some((col) => col.id === id),
    );

    if (invalidCols.length === 0) return;

    addWarning(
      `The following columns are not available: ${invalidCols.join(', ')}`,
    );

    mrtColumnManager.setVisibleColumnIds(
      mrtColumnManager.visibleColumnIds.filter(
        (id) => !invalidCols.includes(id),
      ),
    );
  }, [
    addWarning,
    availableColumns,
    isLoadingFilters,
    isLoadingDevices,
    mrtColumnManager,
  ]);

  return {
    mrtColumnManager,
    warnings,
  };
};
