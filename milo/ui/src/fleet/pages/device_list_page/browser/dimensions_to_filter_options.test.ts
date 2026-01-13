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
  BROWSER_SWARMING_SOURCE,
  BROWSER_UFS_SOURCE,
} from '@/fleet/constants/browser';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { GetBrowserDeviceDimensionsResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { dimensionsToFilterOptions } from './dimensions_to_filter_options';

describe('dimensionsToFilterOptions', () => {
  it('should map dimensions correctly', () => {
    const response = GetBrowserDeviceDimensionsResponse.fromPartial({
      baseDimensions: {
        os: { values: ['linux', 'mac'] },
      },
      swarmingLabels: {
        zone: { values: ['us-west'] },
      },
      ufsLabels: {
        model: { values: ['nuc'] },
      },
    });

    const labelsOverride = {
      os: { headerName: 'OS' },
    };

    const options = dimensionsToFilterOptions(response, labelsOverride);

    expect(options).toHaveLength(3);

    // Check Base Dimension
    const osOption = options.find((o) => o.value === 'os');
    expect(osOption).toBeDefined();
    expect(osOption?.label).toBe('OS');
    expect(osOption?.options).toEqual([
      { label: BLANK_VALUE, value: BLANK_VALUE },
      { label: 'linux', value: 'linux' },
      { label: 'mac', value: 'mac' },
    ]);

    // Check Swarming Label
    const zoneKey = `${BROWSER_SWARMING_SOURCE}."zone"`;
    const zoneOption = options.find((o) => o.value === zoneKey);
    expect(zoneOption).toBeDefined();
    expect(zoneOption?.label).toBe(zoneKey);
    expect(zoneOption?.options).toEqual([
      { label: BLANK_VALUE, value: BLANK_VALUE },
      { label: 'us-west', value: 'us-west' },
    ]);

    // Check UFS Label
    const modelKey = `${BROWSER_UFS_SOURCE}."model"`;
    const modelOption = options.find((o) => o.value === modelKey);
    expect(modelOption).toBeDefined();
    expect(modelOption?.label).toBe(modelKey);
    expect(modelOption?.options).toEqual([
      { label: BLANK_VALUE, value: BLANK_VALUE },
      { label: 'nuc', value: 'nuc' },
    ]);
  });

  it('should NOT filter out dimensions with empty values (it still has the Blank option)', () => {
    const response = GetBrowserDeviceDimensionsResponse.fromPartial({
      baseDimensions: {
        empty: { values: [] },
      },
    });
    const options = dimensionsToFilterOptions(response, {});
    expect(options).toHaveLength(1);
    expect(options[0].options).toHaveLength(1);
    expect(options[0].options[0].value).toBe(BLANK_VALUE);
  });
});
