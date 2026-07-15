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
import { useMemo } from 'react';

import { useMRTColumnManagement } from '@/fleet/components/columns/use_mrt_column_management';
import { FilterCategory } from '@/fleet/components/filters/use_filters';

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

  const mrtColumnManager = useMRTColumnManagement({
    columns: availableColumns,
    defaultColumnIds,
    localStorageKey: 'fleet-console-repairs-columns',
    filterValues,
    isLoadingFilters,
  });

  return {
    mrtColumnManager,
    warnings: mrtColumnManager.warnings,
  };
};
