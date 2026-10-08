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

import { AndroidDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  getAndroidColumnHeader,
  getAndroidColumnOverrides,
} from './android_fields';

describe('getAndroidColumnOverrides', () => {
  const overrides = getAndroidColumnOverrides('Android');

  it('resolves hardware from either hardware or Hardware label', () => {
    const dev = AndroidDevice.fromPartial({
      id: 'pixel-device-1',
      omnilabSpec: { labels: { Hardware: { values: ['android-board-a'] } } },
    });
    expect(overrides.hardware.accessorFn?.(dev)).toBe('android-board-a');
  });

  it('resolves host_version and test_harness with fallbacks', () => {
    const dev = AndroidDevice.fromPartial({
      id: 'host-1',
      omnilabSpec: {
        labels: {
          lab_server_version: { values: ['4.310.0'] },
          dm_type: { values: ['MH'] },
        },
      },
    });
    expect(overrides.host_version.accessorFn?.(dev)).toBe('4.310.0');
    expect(overrides.test_harness.accessorFn?.(dev)).toBe('MH');
  });
});

describe('getAndroidColumnHeader', () => {
  it('returns override headers when defined', () => {
    expect(getAndroidColumnHeader('average_7d')).toBe(
      '7 Day Average Utilization',
    );
    expect(getAndroidColumnHeader('average_30d')).toBe(
      '30 Day Average Utilization',
    );
    expect(getAndroidColumnHeader('fc_offline_since')).toBe('Offline Since');
    expect(getAndroidColumnHeader('id')).toBe('ID');
    expect(getAndroidColumnHeader('location_tag')).toBe('location_tag');
  });

  it('falls back to humanizeColumnLabel for general labels and proto fields', () => {
    expect(getAndroidColumnHeader('battery_level')).toBe('Battery Level');
    expect(getAndroidColumnHeader('host_group')).toBe('Host Group');
    expect(getAndroidColumnHeader('ufs.last_sync')).toBe('UFS Last Sync');
    expect(getAndroidColumnHeader('label-servo_state')).toBe('Servo State');
    expect(getAndroidColumnHeader('realm')).toBe('Realm');
  });
});
