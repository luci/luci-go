// Copyright 2025 The LUCI Authors.
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

import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { BLANK_VALUE } from '@/fleet/constants/filters';

import { dimensionsToFilterOptions } from './helpers';

describe('dimensionsToFilterOptions', () => {
  it('hndles empty', async () => {
    const options = dimensionsToFilterOptions(
      {
        baseDimensions: {},
        labels: {},
      },
      {},
    );

    expect(options).toEqual({});
  });

  it('hadles baseDimensions and labels', async () => {
    const options = dimensionsToFilterOptions(
      {
        baseDimensions: { testDim: { values: ['one', 'two'] } },
        labels: { testLabel: { values: ['a', 'b', 'c'] } },
      },
      {},
    );

    expect(options).toEqual({
      '"testDim"': new StringListFilterCategoryBuilder()
        .setLabel('testDim')
        .setOptions([
          { label: BLANK_VALUE, value: BLANK_VALUE },
          { label: 'one', value: '"one"' },
          { label: 'two', value: '"two"' },
        ]),
      'labels."testLabel"': new StringListFilterCategoryBuilder()
        .setLabel('testLabel')
        .setOptions([
          { label: BLANK_VALUE, value: BLANK_VALUE },
          { label: 'a', value: '"a"' },
          { label: 'b', value: '"b"' },
          { label: 'c', value: '"c"' },
        ]),
    });
  });
});
