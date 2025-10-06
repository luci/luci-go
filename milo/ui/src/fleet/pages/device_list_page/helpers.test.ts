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

import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { dimensionsToFilterOptions } from './helpers';

describe('dimensionsToFilterOptions', () => {
  it('hndles empty', async () => {
    const options = dimensionsToFilterOptions(
      {
        baseDimensions: {},
        labels: {},
      },
      Platform.UNSPECIFIED,
    );

    expect(options).toEqual([]);
  });

  it('hadles baseDimensions and labels', async () => {
    const options = dimensionsToFilterOptions(
      {
        baseDimensions: { testDim: { values: ['one', 'two'] } },
        labels: { testLabel: { values: ['a', 'b', 'c'] } },
      },
      Platform.UNSPECIFIED,
    );

    expect(options).toEqual([
      {
        label: 'testDim',
        value: 'testDim',
        options: [
          { label: 'one', value: 'one' },
          { label: 'two', value: 'two' },
        ],
      },
      {
        label: 'testLabel',
        value: 'labels."testLabel"',
        options: [
          { label: 'a', value: 'a' },
          { label: 'b', value: 'b' },
          { label: 'c', value: 'c' },
        ],
      },
    ]);
  });
});
