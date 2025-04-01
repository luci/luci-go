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

import { GridColumnVisibilityModel } from '@mui/x-data-grid';

import { getVisibilityModel } from './search_param_utils';

describe('data_table_search_param_utils', () => {
  describe('getVisibilityModel', () => {
    it('should work in the basic case', () => {
      const newColumnVisibilityModel: GridColumnVisibilityModel = {
        column1: true,
        column2: false,
        column3: false,
      };

      expect(
        getVisibilityModel(['column1', 'column2', 'column3'], ['column1']),
      ).toEqual(newColumnVisibilityModel);
    });
    it('should ignore columns set to visible that dont really exists', () => {
      const newColumnVisibilityModel: GridColumnVisibilityModel = {
        column1: true,
        column2: false,
        column3: false,
      };

      expect(
        getVisibilityModel(
          ['column1', 'column2', 'column3'],
          ['column1', 'potato'],
        ),
      ).toEqual(newColumnVisibilityModel);
    });
  });
});
