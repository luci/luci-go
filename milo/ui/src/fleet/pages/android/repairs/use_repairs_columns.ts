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

import { MRT_ColumnDef } from 'material-react-table';
import { useMemo, useEffect } from 'react';

import {
  useMRTColumnManagement,
  getColumnId,
} from '@/fleet/components/columns/use_mrt_column_management';
import { FilterCategory } from '@/fleet/components/filters/use_filters';
import { useWarnings } from '@/fleet/utils/use_warnings';

import { COLUMNS } from './repairs_columns';
import { Row } from './repairs_columns.utils';

export const useRepairsColumns = (
  filterValues: Record<string, FilterCategory> | undefined,
  isLoadingFilters: boolean,
) => {
  const availableColumns = useMemo<MRT_ColumnDef<Row>[]>(
    () => Object.values(COLUMNS),
    [],
  );
  const defaultColumnIds = useMemo(() => Object.keys(COLUMNS), []);

  // TODO(b/524930584): This block (calculating highlighted column IDs based on active filters)
  // is conceptually identical across ChromeOS, Browser, and RRI column hooks, and could be unified.
  const highlightedColumnIds = useMemo(
    () =>
      filterValues === undefined
        ? []
        : Object.entries(filterValues)
            .filter(([_key, filter]) => filter.isActive())
            .map(([key]) => key),
    [filterValues],
  );

  const mrtColumnManager = useMRTColumnManagement({
    columns: availableColumns,
    defaultColumnIds,
    localStorageKey: 'fleet-console-repairs-columns',
    highlightedColumnIds,
  });

  const [warnings, addWarning] = useWarnings();

  // TODO(b/524930584): This validation and warning synchronizer is identical across ChromeOS,
  // Browser, and RRI column hooks, and could be moved to a shared hook or integrated into useMRTColumnManagement.
  // Validates the column search parameters and alerts the user of any invalid columns.
  // This is purely for validation and UI alert purposes. Syncing of columns between
  // the URL and local state is handled entirely inside useMRTColumnManagement.
  useEffect(() => {
    if (isLoadingFilters) return;

    const invalidCols = mrtColumnManager.visibleColumnIds.filter(
      (id) => !availableColumns.some((col) => getColumnId(col) === id),
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
  }, [addWarning, availableColumns, isLoadingFilters, mrtColumnManager]);

  return {
    mrtColumnManager,
    warnings,
  };
};
