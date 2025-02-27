// Copyright 2023 The LUCI Authors.
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

import { GridColDef, GridColumnVisibilityModel } from '@mui/x-data-grid';

import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';

const areVisibilityModelsEqual = (
  a: GridColumnVisibilityModel,
  b: GridColumnVisibilityModel,
) => {
  const keys = Object.keys(a);
  return (
    keys.length === Object.keys(a).length &&
    keys.every((key) => key in b && a[key] === b[key])
  );
};

export function getVisibleColumns(
  params: URLSearchParams,
  defaultColumnVisibilityModel: GridColumnVisibilityModel,
  allColumns: GridColDef[],
) {
  const visibleColumns = params.getAll(COLUMNS_PARAM_KEY);
  return visibleColumns.length !== 0
    ? allColumns.reduce(
        (visibilityModel, column) => ({
          ...visibilityModel,
          [column.field]: visibleColumns.includes(column.field),
        }),
        {} as GridColumnVisibilityModel,
      )
    : defaultColumnVisibilityModel;
}

export function visibleColumnsUpdater(
  newColumnVisibilityModel: GridColumnVisibilityModel,
  defaultColumnVisibilityModel: GridColumnVisibilityModel,
) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (
      areVisibilityModelsEqual(
        newColumnVisibilityModel,
        defaultColumnVisibilityModel,
      )
    ) {
      searchParams.delete(COLUMNS_PARAM_KEY);
    } else {
      Object.entries(newColumnVisibilityModel).forEach(
        ([column, isVisible]) => {
          if (isVisible && !searchParams.has(COLUMNS_PARAM_KEY, column)) {
            searchParams.append(COLUMNS_PARAM_KEY, column);
          } else if (
            !isVisible &&
            searchParams.has(COLUMNS_PARAM_KEY, column)
          ) {
            searchParams.delete(COLUMNS_PARAM_KEY, column);
          }
        },
      );
    }
    return searchParams;
  };
}
