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

import { BLANK_VALUE } from '@/fleet/constants/filters';
import { StringListCategory } from '@/fleet/types';
import { GetRepairMetricsDimensionsResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { dimensionsToFilterOptions } from './repairs_page_utils';

describe('dimensionsToFilterOptions', () => {
  it('should include BLANK_VALUE in options', () => {
    const mockResponse: GetRepairMetricsDimensionsResponse = {
      dimensions: {
        pool: { values: ['pool1', 'pool2'] },
        os: { values: ['os1'] },
      },
    };

    const options = dimensionsToFilterOptions(mockResponse);

    expect(options).toHaveLength(2);

    const poolOption = options.find(
      (o) => o.value === 'pool',
    ) as StringListCategory;
    expect(poolOption).toBeDefined();
    expect(poolOption?.options).toContainEqual({
      label: BLANK_VALUE,
      value: BLANK_VALUE,
    });
    expect(poolOption?.options).toContainEqual({
      label: 'pool1',
      value: 'pool1',
    });
    expect(poolOption?.options).toContainEqual({
      label: 'pool2',
      value: 'pool2',
    });

    const osOption = options.find(
      (o) => o.value === 'os',
    ) as StringListCategory;
    expect(osOption).toBeDefined();
    expect(osOption?.options).toContainEqual({
      label: BLANK_VALUE,
      value: BLANK_VALUE,
    });
    expect(osOption?.options).toContainEqual({ label: 'os1', value: 'os1' });
  });

  it('should handle empty dimensions', () => {
    const mockResponse: GetRepairMetricsDimensionsResponse = {
      dimensions: {},
    };

    const options = dimensionsToFilterOptions(mockResponse);

    expect(options).toHaveLength(0);
  });

  it('should not include duplicates for blank values', () => {
    const mockResponse: GetRepairMetricsDimensionsResponse = {
      dimensions: {
        pool: { values: ['pool1', '', BLANK_VALUE] },
      },
    };

    const options = dimensionsToFilterOptions(mockResponse);
    const poolOption = options.find(
      (o) => o.value === 'pool',
    ) as StringListCategory;

    expect(poolOption).toBeDefined();
    // Should have BLANK_VALUE (added by default) and 'pool1'.
    // '' and BLANK_VALUE from data should be filtered out.
    expect(poolOption?.options).toHaveLength(2);
    expect(poolOption?.options).toContainEqual({
      label: BLANK_VALUE,
      value: BLANK_VALUE,
    });
    expect(poolOption?.options).toContainEqual({
      label: 'pool1',
      value: 'pool1',
    });
  });
});
