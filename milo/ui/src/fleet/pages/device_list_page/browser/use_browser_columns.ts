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
import { BROWSER_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { BROWSER_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useWarnings } from '@/fleet/utils/use_warnings';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';

import { getBrowserColumn, getBrowserColumnIds } from './browser_columns';
import { useBrowserDeviceDimensions } from './use_browser_device_dimensions';

export const useBrowserColumns = (
  filterValues: Record<string, FilterCategory> | undefined,
  isLoadingFilters: boolean,
  isLoadingDevices: boolean,
) => {
  const dimensionsQuery = useBrowserDeviceDimensions();
  const [searchParams] = useSyncedSearchParams();

  const urlCols = useMemo(() => {
    const columnsParamStr = searchParams.getAll(COLUMNS_PARAM_KEY).join(',');
    return columnsParamStr ? columnsParamStr.split(',') : [];
  }, [searchParams]);

  const availableColumns = useMemo(() => {
    const requiredCols = urlCols.length > 0 ? urlCols : BROWSER_DEFAULT_COLUMNS;
    const columnIds = getBrowserColumnIds(dimensionsQuery.data, requiredCols);
    return columnIds.map((id) => getBrowserColumn(id));
  }, [dimensionsQuery.data, urlCols]);

  // TODO(b/524930584): This block (mapping filter keys to column IDs) is identical across
  // ChromeOS, Browser, and RRI column hooks, and is a strong candidate for a shared utility.
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

  // TODO(b/524930584): This block (calculating highlighted column IDs based on active filters)
  // is identical across ChromeOS, Browser, and RRI column hooks, and is a strong candidate for a shared utility.
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
    defaultColumnIds: BROWSER_DEFAULT_COLUMNS,
    localStorageKey: BROWSER_DEVICES_LOCAL_STORAGE_KEY,
    highlightedColumnIds,
    platform: Platform.CHROMIUM,
  });

  const [warnings, addWarning] = useWarnings();

  // TODO(b/524930584): This validation and warning synchronizer is identical across ChromeOS,
  // Browser, and RRI column hooks, and could be moved to a shared hook or integrated into useMRTColumnManagement.
  // Validates the column search parameters and alerts the user of any invalid columns.
  // This is purely for validation and UI alert purposes. Syncing of columns between
  // the URL and local state is handled entirely inside useMRTColumnManagement.
  useEffect(() => {
    if (dimensionsQuery.isPending || isLoadingFilters || isLoadingDevices)
      return;

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
    dimensionsQuery.isPending,
    isLoadingFilters,
    isLoadingDevices,
    mrtColumnManager,
  ]);

  return {
    mrtColumnManager,
    warnings,
  };
};
