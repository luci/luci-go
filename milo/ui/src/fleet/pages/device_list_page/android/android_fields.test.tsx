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

import { getAndroidColumnOverrides } from './android_fields';

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
