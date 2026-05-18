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

import { CHROMEOS_DEFAULT_COLUMNS } from '@/fleet/config/device_config';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useWarnings } from '@/fleet/utils/use_warnings';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { EXTRA_COLUMN_IDS } from './chromeos_fields';
import { useChromeOSFields } from './use_chromeos_available_columns';

export const useChromeOSColumns = (
  isLoadingFilters: boolean,
  isLoadingDevices: boolean,
) => {
  const { availableFields } = useChromeOSFields();

  const availableColumns = useMemo(() => {
    return availableFields.map((def) => def.columnDef);
  }, [availableFields]);

  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const visibleColumnIds = useMemo(() => {
    const columnsParamStr = searchParams.getAll(COLUMNS_PARAM_KEY).join(',');
    const urlCols = columnsParamStr ? columnsParamStr.split(',') : [];
    const requiredCols =
      urlCols.length > 0 ? urlCols : CHROMEOS_DEFAULT_COLUMNS;

    return _.uniq([...requiredCols, ...EXTRA_COLUMN_IDS]);
  }, [searchParams]);

  const visibleColumns = useMemo(() => {
    return availableColumns.filter((col) => visibleColumnIds.includes(col.id));
  }, [availableColumns, visibleColumnIds]);

  const [warnings, addWarning] = useWarnings();
  useEffect(() => {
    if (isLoadingFilters || isLoadingDevices) return;

    const invalidCols = visibleColumnIds.filter(
      (id) => !visibleColumns.some((col) => col.id === id),
    );
    if (invalidCols.length === 0) return;
    addWarning(
      `The following columns are not available: ${invalidCols.join(', ')}`,
    );
    for (const col of invalidCols) {
      searchParams.delete(COLUMNS_PARAM_KEY, col);
    }
    if (searchParams.getAll(COLUMNS_PARAM_KEY).length <= 1)
      searchParams.delete(COLUMNS_PARAM_KEY);

    setSearchParams(searchParams);
  }, [
    addWarning,
    visibleColumnIds,
    isLoadingFilters,
    isLoadingDevices,
    searchParams,
    setSearchParams,
    availableColumns,
    visibleColumns,
  ]);

  return {
    availableColumns,
    visibleColumns,
    warnings,
  };
};
