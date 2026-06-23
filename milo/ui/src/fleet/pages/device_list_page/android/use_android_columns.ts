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

import _ from 'lodash';
import { useMemo, useEffect } from 'react';

import { useMRTColumnManagement } from '@/fleet/components/columns/use_mrt_column_management';
import { normalizeFilterKey } from '@/fleet/components/filters/normalize_filter_key';
import { FilterCategory } from '@/fleet/components/filters/use_filters';
import { ANDROID_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { ANDROID_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { useDeviceDimensions } from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { useWarnings } from '@/fleet/utils/use_warnings';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getAndroidColumns } from './android_columns';

export const useAndroidColumns = (
  filterValues: Record<string, FilterCategory> | undefined,
  isLoadingFilters: boolean,
  isLoadingDevices: boolean,
  showAvgUtilization: boolean,
) => {
  const dimensionsQuery = useDeviceDimensions({ platform: Platform.ANDROID });

  const extraColumnIds = useMemo(() => {
    const ids = [
      'label-id',
      'label-run_target',
      'label-hostname',
      'fc_offline_since',
      'realm',
    ];
    if (showAvgUtilization) {
      ids.push('average_7d', 'average_30d');
    }
    return ids;
  }, [showAvgUtilization]);

  const availableColumns = useMemo(() => {
    const list: { id: string; label: string }[] = [];

    ANDROID_DEFAULT_COLUMNS.forEach((id) => list.push({ id, label: id }));
    extraColumnIds.forEach((id) => {
      let label = id;
      if (id === 'average_7d') label = '7 Day Average Utilization';
      if (id === 'average_30d') label = '30 Day Average Utilization';
      list.push({ id, label });
    });

    if (dimensionsQuery.data) {
      Object.keys(dimensionsQuery.data.baseDimensions).forEach((id) =>
        list.push({ id, label: id }),
      );
      Object.keys(dimensionsQuery.data.labels).forEach((id) =>
        list.push({ id, label: id }),
      );
    }

    return _.uniqBy(list, 'id');
  }, [extraColumnIds, dimensionsQuery.data]);

  const allColumns = useMemo(() => {
    return getAndroidColumns(availableColumns.map((c) => c.id));
  }, [availableColumns]);

  // TODO(b/524930584): This block (mapping filter keys to column IDs) is identical across
  // ChromeOS, Browser, and RRI column hooks, and is a strong candidate for a shared utility.
  const { filterByFieldToId } = useMemo(() => {
    const fromFieldId = new Map<string, string>();
    allColumns.forEach((c) => {
      const cId = c.id;
      const filterKey = c.filterKey || cId;
      if (filterKey && cId) {
        fromFieldId.set(filterKey, cId);
      }
    });
    return { filterByFieldToId: fromFieldId };
  }, [allColumns]);

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
    columns: allColumns,
    defaultColumnIds: ANDROID_DEFAULT_COLUMNS,
    localStorageKey: ANDROID_DEVICES_LOCAL_STORAGE_KEY,
    highlightedColumnIds,
    platform: Platform.ANDROID,
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
      (id) => !allColumns.some((col) => col.id === id),
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
    allColumns,
    dimensionsQuery.isPending,
    isLoadingFilters,
    isLoadingDevices,
    mrtColumnManager,
  ]);

  return {
    mrtColumnManager,
    warnings,
    availableColumns,
  };
};
