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
import { useMemo } from 'react';

import { useMRTColumnManagement } from '@/fleet/components/columns/use_mrt_column_management';
import { FilterCategory } from '@/fleet/components/filters/use_filters';
import { ANDROID_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { ANDROID_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { useDeviceDimensions } from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getAndroidColumns } from './android_columns';

const EXTRA_COLUMN_IDS = [
  'label-id',
  'label-run_target',
  'label-hostname',
  'fc_offline_since',
  'realm',
];

export const useAndroidColumns = (
  filterValues: Record<string, FilterCategory> | undefined,
  isLoadingFilters: boolean,
  isLoadingDevices: boolean,
  showAvgUtilization = false,
) => {
  const dimensionsQuery = useDeviceDimensions({ platform: Platform.ANDROID });

  const extraColumnIds = useMemo(() => {
    const ids = [...EXTRA_COLUMN_IDS];
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

  const mrtColumnManager = useMRTColumnManagement({
    columns: allColumns,
    defaultColumnIds: ANDROID_DEFAULT_COLUMNS,
    localStorageKey: ANDROID_DEVICES_LOCAL_STORAGE_KEY,
    filterValues,
    isLoadingColumns: dimensionsQuery.isPending,
    isLoadingFilters,
    isLoadingDevices,
    platform: Platform.ANDROID,
  });

  return {
    mrtColumnManager,
    warnings: mrtColumnManager.warnings,
    availableColumns,
  };
};
