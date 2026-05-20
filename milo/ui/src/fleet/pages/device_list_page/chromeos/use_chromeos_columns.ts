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

import { EXTRA_COLUMN_IDS, getFieldDefinition } from './chromeos_fields';
import { useChromeOSFields } from './use_chromeos_available_columns';

export const useChromeOSColumns = (
  isLoadingFilters: boolean,
  isLoadingDevices: boolean,
) => {
  const { availableFields } = useChromeOSFields();

  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const urlCols = useMemo(() => {
    const columnsParamStr = searchParams.getAll(COLUMNS_PARAM_KEY).join(',');
    return columnsParamStr ? columnsParamStr.split(',') : [];
  }, [searchParams]);

  const visibleColumnIds = useMemo(() => {
    const requiredCols =
      urlCols.length > 0 ? urlCols : CHROMEOS_DEFAULT_COLUMNS;

    return _.uniq([...requiredCols, ...EXTRA_COLUMN_IDS]);
  }, [urlCols]);

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

  const visibleColumns = useMemo(() => {
    return availableColumns.filter((col) => visibleColumnIds.includes(col.id));
  }, [availableColumns, visibleColumnIds]);

  const [warnings, addWarning] = useWarnings();
  useEffect(() => {
    if (isLoadingFilters || isLoadingDevices) return;

    // Note: Since availableColumns (and thus visibleColumns) now includes
    // missing columns from visibleColumnIds as fallbacks, invalid columns will
    // be filtered out on the first render, executing setSearchParams exactly once
    // and avoiding infinite loops.
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
