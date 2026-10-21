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

import { OptionValue } from '@/fleet/types/option';
import { fuzzySort } from '@/fleet/utils/fuzzy_sort';
import { GetBrowserDeviceDimensionsResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getBrowserColumn, getBrowserColumnIds } from './browser_columns';

describe('browser_columns', () => {
  it('humanizes swarming, UFS, custom, and base dimension headers', () => {
    expect(getBrowserColumn('sw.os').header).toBe('SW OS');
    expect(getBrowserColumn('sw.current_task').header).toBe('SW Current Task');
    expect(getBrowserColumn('sw.device_type').header).toBe('SW Device Type');
    expect(getBrowserColumn('sw.dut_state').header).toBe('SW DUT State');
    expect(getBrowserColumn('sw.state').header).toBe('SW State');
    expect(getBrowserColumn('sw.pool').header).toBe('SW Pool');
    expect(getBrowserColumn('ufs.resource_state').header).toBe(
      'UFS Resource State',
    );
    expect(getBrowserColumn('ufs.zone').header).toBe('UFS Zone');
    expect(getBrowserColumn('ufs.serial_number').header).toBe(
      'UFS Serial Number',
    );
    expect(getBrowserColumn('ufs.chrome_platform').header).toBe(
      'UFS Chrome Platform',
    );
    expect(getBrowserColumn('id').header).toBe('Machine');
    expect(getBrowserColumn('ufs.hostname').header).toBe('Host / Bot ID');
    expect(getBrowserColumn('unhealthy_devices_ratio').header).toBe(
      'Unhealthy / Total Devices',
    );
    expect(getBrowserColumn('realm').header).toBe('Realm');
    expect(getBrowserColumn('device_type').header).toBe('Device Type');
    expect(getBrowserColumn('model').header).toBe('Model');
    expect(getBrowserColumn('gpu').header).toBe('GPU');
  });

  it('supports column search by humanized label and raw backend key', () => {
    const columns: OptionValue[] = [
      'id',
      'ufs.hostname',
      'unhealthy_devices_ratio',
      'sw.os',
      'ufs.resource_state',
      'realm',
    ].map((id) => {
      const col = getBrowserColumn(id);
      return {
        label: typeof col.header === 'string' ? col.header : id,
        value: id,
      };
    });
    const search = (q: string) =>
      fuzzySort(q)(
        columns,
        (x: OptionValue) => x.label,
        (x: OptionValue) => x.value,
      ).filter((item) => item.score > 0);

    for (const [query, expected] of [
      ['SW OS', 'sw.os'],
      ['UFS Resource State', 'ufs.resource_state'],
      ['Machine', 'id'],
      ['sw.os', 'sw.os'],
      ['resource_state', 'ufs.resource_state'],
      ['ufs.resource_state', 'ufs.resource_state'],
      ['hostname', 'ufs.hostname'],
      ['host/bot_id', 'ufs.hostname'],
      ['unhealthy_devices_ratio', 'unhealthy_devices_ratio'],
      ['unhealthy/total', 'unhealthy_devices_ratio'],
    ]) {
      expect(search(query).some((r) => r.el.value === expected)).toBe(true);
    }
  });

  it('returns all dimension and custom columns uniquely', () => {
    const dimensions = GetBrowserDeviceDimensionsResponse.fromPartial({
      baseDimensions: { id: {}, model: {} },
      swarmingLabels: { os: {} },
      ufsLabels: { resource_state: {} },
    });
    const ids = getBrowserColumnIds(dimensions, ['custom_extra']);
    expect(ids).toEqual(
      expect.arrayContaining([
        'model',
        'sw.os',
        'ufs.resource_state',
        'unhealthy_devices_ratio',
        'id',
        'realm',
        'ufs.location_group_tag',
        'custom_extra',
      ]),
    );
  });
});
