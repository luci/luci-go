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

import { getVisibleColumns, visibleColumnsUpdater } from './search_param_utils';

const DEFAULT_COLUMN_VISIBILITY_MODEL: GridColumnVisibilityModel = {
  column1: true,
  column2: true,
  column3: false,
};

const ALL_COLUMNS: GridColDef[] = [
  { field: 'column1' },
  { field: 'column2' },
  { field: 'column3' },
];

describe('data_table_search_param_utils', () => {
  describe('getVisibleColumns', () => {
    it('should return default model when no columns are specified in URL', () => {
      expect(
        getVisibleColumns(
          new URLSearchParams(''),
          DEFAULT_COLUMN_VISIBILITY_MODEL,
          ALL_COLUMNS,
        ),
      ).toEqual(DEFAULT_COLUMN_VISIBILITY_MODEL);
    });

    it('should return visibility model based on URL params', () => {
      const expectedVisibilityModel: GridColumnVisibilityModel = {
        column1: true,
        column2: false,
        column3: true,
      };
      expect(
        getVisibleColumns(
          new URLSearchParams('c=column1&c=column3'),
          DEFAULT_COLUMN_VISIBILITY_MODEL,
          ALL_COLUMNS,
        ),
      ).toEqual(expectedVisibilityModel);
    });

    it('should handle URL params with non-existent columns', () => {
      const expectedVisibilityModel: GridColumnVisibilityModel = {
        column1: true,
        column2: false,
        column3: false,
      };
      expect(
        getVisibleColumns(
          new URLSearchParams('c=column1&c=nonexistent'),
          DEFAULT_COLUMN_VISIBILITY_MODEL,
          ALL_COLUMNS,
        ),
      ).toEqual(expectedVisibilityModel);
    });
  });

  describe('visibleColumnsUpdater', () => {
    it('should delete columns param when new model equals default', () => {
      const newColumnVisibilityModel: GridColumnVisibilityModel = {
        column1: true,
        column2: true,
        column3: false,
      };
      const updatedParams = visibleColumnsUpdater(
        newColumnVisibilityModel,
        DEFAULT_COLUMN_VISIBILITY_MODEL,
      )(new URLSearchParams('c=column1&c=column2'));
      expect(updatedParams.toString()).toBe('');
    });
  });
});
