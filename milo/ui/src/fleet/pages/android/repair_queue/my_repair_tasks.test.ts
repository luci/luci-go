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

import { FILTERS_PARAM_KEY } from '@/fleet/constants/param_keys';
import {
  AndroidMachineType,
  AndroidRepairQueueItem,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  getClaimedByFilter,
  getDeviceListLinkForRepairTasks,
} from './my_repair_tasks';

const item = (
  deviceId: string,
  fcMachineType: AndroidMachineType,
): AndroidRepairQueueItem =>
  AndroidRepairQueueItem.fromPartial({ deviceId, fcMachineType });

const DEVICE = AndroidMachineType.ANDROID_MACHINE_TYPE_DEVICE;
const HOST = AndroidMachineType.ANDROID_MACHINE_TYPE_HOST;

const filterOf = (link: string) =>
  new URL(link, 'https://example.com').searchParams.get(FILTERS_PARAM_KEY);

describe('my_repair_tasks', () => {
  it('builds a quoted claimed_by filter', () => {
    expect(getClaimedByFilter('tech@example.com')).toBe(
      'claimed_by = "tech@example.com"',
    );
    expect(getClaimedByFilter('a"b')).toBe('claimed_by = "a\\"b"');
  });

  it('returns an empty device list link without tasks', () => {
    expect(getDeviceListLinkForRepairTasks('Android', [])).toBe('');
  });

  it('filters the device list by deduplicated ids', () => {
    const link = getDeviceListLinkForRepairTasks('Pixel', [
      item('d1', DEVICE),
      item('d2', DEVICE),
      item('d1', HOST),
    ]);
    expect(link.startsWith('/ui/fleet/p/pixel/devices?')).toBe(true);
    const filter = filterOf(link);
    expect(filter).toContain('"id"');
    expect(filter).toContain('"d1"');
    expect(filter).toContain('"d2"');
    expect(filter?.match(/"d1"/g)).toHaveLength(1);
    expect(filter).not.toContain('fc_machine_type');
  });
});
