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
import { CHROMEOS_PLATFORM } from '@/fleet/constants/paths';

import {
  buildRepairTasksDeviceListLink,
  getMyClaimedRepairTasks,
} from './my_repair_tasks';
import { UNKNOWN_USER_EMAIL } from './use_current_user_email';

const filterOf = (link: string) =>
  new URL(link, 'https://example.com').searchParams.get(FILTERS_PARAM_KEY);

describe('getMyClaimedRepairTasks', () => {
  it('excludes unclaimed, other-user, and blank-deviceId tasks while preserving shared host/device IDs', () => {
    const items = [
      { deviceId: 'host-1', claimedBy: 'operator1@example.com' },
      { deviceId: 'shared', claimedBy: 'operator1@example.com' },
      { deviceId: 'shared', claimedBy: 'operator1@example.com' },
      { deviceId: '', claimedBy: 'operator1@example.com' },
      { deviceId: '   ', claimedBy: 'operator1@example.com' },
      { deviceId: 'host-2', claimedBy: 'other@example.com' },
      { deviceId: 'host-3', claimedBy: '' },
    ];

    expect(getMyClaimedRepairTasks(items, 'operator1@example.com')).toEqual([
      { deviceId: 'host-1', claimedBy: 'operator1@example.com' },
      { deviceId: 'shared', claimedBy: 'operator1@example.com' },
      { deviceId: 'shared', claimedBy: 'operator1@example.com' },
    ]);
    expect(getMyClaimedRepairTasks(items, UNKNOWN_USER_EMAIL)).toEqual([]);
    expect(getMyClaimedRepairTasks(items, '')).toEqual([]);
  });
});

describe('buildRepairTasksDeviceListLink', () => {
  it('returns an empty link when there are no valid device IDs', () => {
    expect(buildRepairTasksDeviceListLink([], CHROMEOS_PLATFORM)).toBe('');
    expect(
      buildRepairTasksDeviceListLink(
        [{ deviceId: '' }, { deviceId: '   ' }],
        CHROMEOS_PLATFORM,
      ),
    ).toBe('');
  });

  it('builds unquoted id filter for ChromeOS and deduplicates IDs', () => {
    const link = buildRepairTasksDeviceListLink(
      [
        { deviceId: 'demo-cros-r2-rk1-h2' },
        { deviceId: 'host-1' },
        { deviceId: 'demo-cros-r2-rk1-h2' },
      ],
      CHROMEOS_PLATFORM,
    );
    expect(link.startsWith('/ui/fleet/p/chromeos/devices?')).toBe(true);
    expect(filterOf(link)).toBe(
      '(id = "demo-cros-r2-rk1-h2" OR id = "host-1")',
    );
  });

  it('supports quoted id filter keys and deduplicates shared IDs for Android/Pixel workspaces', () => {
    const link = buildRepairTasksDeviceListLink(
      [{ deviceId: 'd1' }, { deviceId: 'd2' }, { deviceId: 'd1' }],
      'pixel',
      '"id"',
    );
    expect(link.startsWith('/ui/fleet/p/pixel/devices?')).toBe(true);
    const filter = filterOf(link);
    expect(filter).toBe('("id" = "d1" OR "id" = "d2")');
    expect(filter).not.toContain('fc_machine_type');
  });
});
