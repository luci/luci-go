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

import { StringListFilterCategory } from './string_list_filter';

describe('StringListFilterCategory', () => {
  it('should get selected options', () => {
    const category = new StringListFilterCategory(
      'Model',
      'model',
      [
        { value: '"pixel"', label: 'Pixel' },
        { value: '"nexus"', label: 'Nexus' },
      ],
      [],
      () => {},
      [],
    );

    category.setSelectedOptions(['pixel']);
    expect(category.getSelectedOptions()).toEqual(['"pixel"']);
  });

  it('should set selected options correctly with quotes', () => {
    const category = new StringListFilterCategory(
      'Model',
      'model',
      [
        { value: '"pixel"', label: 'Pixel' },
        { value: '"nexus"', label: 'Nexus' },
      ],
      [],
      () => {},
      [],
    );

    category.setSelectedOptions(['pixel']);
    expect(category.getSelectedOptions()).toEqual(['"pixel"']);
  });

  it('should handle unquoted keys for unquoted options', () => {
    const category = new StringListFilterCategory(
      'Model',
      'model',
      [
        { value: 'pixel', label: 'Pixel' },
        { value: 'nexus', label: 'Nexus' },
      ],
      [],
      () => {},
      [],
    );

    category.setSelectedOptions(['pixel']);
    expect(category.getSelectedOptions()).toEqual(['pixel']);
  });
  it('should handle internal quotes in keys', () => {
    const category = new StringListFilterCategory(
      'Model',
      'model',
      [{ value: '"device \\"pro\\" model"', label: 'Pro Model' }],
      [],
      () => {},
      [],
    );

    category.setSelectedOptions(['device \\"pro\\" model']);
    expect(category.getSelectedOptions()).toEqual(['"device \\"pro\\" model"']);
  });

  it('should clear all options when an empty array is passed', () => {
    const category = new StringListFilterCategory(
      'Model',
      'model',
      [
        { value: '"pixel"', label: 'Pixel' },
        { value: '"nexus"', label: 'Nexus' },
      ],
      [],
      () => {},
      [],
    );

    category.setSelectedOptions(['pixel']);
    expect(category.getSelectedOptions()).toEqual(['"pixel"']);

    category.setSelectedOptions([]);
    expect(category.getSelectedOptions()).toEqual([]);
  });
});
