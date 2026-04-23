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

import {
  StringListFilterCategory,
  StringListFilterCategoryBuilder,
} from '@/fleet/components/filters/string_list_filter';
import { FilterCategory } from '@/fleet/components/filters/use_filters';

import { computeSelectedOptions, syncFilterCategory } from './filters';

describe('filters utility', () => {
  describe('computeSelectedOptions', () => {
    it('should return empty object when filterValues is undefined', () => {
      expect(computeSelectedOptions(undefined)).toEqual({});
    });

    it('should return empty object when no filters are active', () => {
      const mockCategory = {
        isActive: () => false,
      } as unknown as FilterCategory;

      expect(computeSelectedOptions({ testKey: mockCategory })).toEqual({});
    });

    it('should extract selected options from StringListFilterCategory', () => {
      const builder = new StringListFilterCategoryBuilder()
        .setLabel('Label')
        .setOptions([
          { label: 'value1', value: 'value1' },
          { label: 'value2', value: 'value2' },
        ]);

      const buildResult = builder.build('testKey', () => {}, null);
      if (buildResult.isError) throw new Error(buildResult.error);
      const category = buildResult.value;
      category.setSelectedOptions(['value1', 'value2']);

      const result = computeSelectedOptions({ testKey: category });

      expect(result).toEqual({
        testKey: ['value1', 'value2'],
      });
    });
  });

  describe('syncFilterCategory', () => {
    it('should do nothing if filter is not in new filters and was not in table', () => {
      const mockCategory = {
        getSelectedOptions: jest.fn(),
      } as unknown as StringListFilterCategory;
      Object.setPrototypeOf(mockCategory, StringListFilterCategory.prototype);

      syncFilterCategory('testKey', mockCategory, {}, []);

      expect(mockCategory.getSelectedOptions).not.toHaveBeenCalled();
    });

    it('should set selected options when filter is in new filters', () => {
      const builder = new StringListFilterCategoryBuilder()
        .setLabel('Label')
        .setOptions([{ label: 'value1', value: 'value1' }]);

      const buildResult = builder.build('testKey', () => {}, null);
      if (buildResult.isError) throw new Error(buildResult.error);
      const category = buildResult.value;
      const spy = jest.spyOn(category, 'setSelectedOptions');

      syncFilterCategory('testKey', category, { testKey: ['value1'] }, []);

      expect(spy).toHaveBeenCalledWith(['value1'], true);
    });
  });
});
