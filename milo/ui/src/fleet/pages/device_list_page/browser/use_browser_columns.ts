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

import { useMRTColumnManagement } from '@/fleet/components/columns/use_mrt_column_management';
import { FilterCategory } from '@/fleet/components/filters/use_filters';
import { BROWSER_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { BROWSER_DEVICES_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
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

  const mrtColumnManager = useMRTColumnManagement({
    columns: availableColumns,
    defaultColumnIds: BROWSER_DEFAULT_COLUMNS,
    localStorageKey: BROWSER_DEVICES_LOCAL_STORAGE_KEY,
    filterValues,
    isLoadingColumns: dimensionsQuery.isPending,
    isLoadingFilters,
    isLoadingDevices,
    platform: Platform.CHROMIUM,
  });

  return {
    mrtColumnManager,
    warnings: mrtColumnManager.warnings,
  };
};
