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

import { humanizeColumnLabel } from './humanize_column';

describe('humanizeColumnLabel', () => {
  it('humanizes raw label-* and snake_case keys while preserving acronyms', () => {
    expect(humanizeColumnLabel('id')).toBe('ID');
    expect(humanizeColumnLabel('dut_id')).toBe('DUT ID');
    expect(humanizeColumnLabel('dut_state')).toBe('DUT State');
    expect(humanizeColumnLabel('label-servo_state')).toBe('Servo State');
    expect(humanizeColumnLabel('label-board')).toBe('Board');
    expect(humanizeColumnLabel('label-os_type')).toBe('OS Type');
    expect(humanizeColumnLabel('sw_version')).toBe('SW Version');
    expect(humanizeColumnLabel('ufs_zone')).toBe('UFS Zone');
    expect(humanizeColumnLabel('AssociatedHostname')).toBe(
      'Associated Hostname',
    );
    expect(humanizeColumnLabel('Servo State')).toBe('Servo State');
    expect(humanizeColumnLabel('dms.pool')).toBe('DMS Pool');
    expect(humanizeColumnLabel('ufs.last_sync')).toBe('UFS Last Sync');
    expect(humanizeColumnLabel('mh.last_sync')).toBe('MH Last Sync');
  });

  it('handles empty strings gracefully', () => {
    expect(humanizeColumnLabel('')).toBe('');
  });
});
