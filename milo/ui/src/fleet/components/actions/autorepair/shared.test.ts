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

import {
  ScheduleAutorepairRequest,
  ScheduleAutorepairRequest_AutorepairFlag,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  autorepairRequestsFromDuts,
  createScheduleAutorepairRequest,
  formatShivasRepairCommand,
  REPAIR_DEEP_TASK_NAME,
  REPAIR_TASK_NAME,
  TASK_PRIORITY,
} from './shared';

jest.mock('@/fleet/utils/builds', () => ({
  ...jest.requireActual('@/fleet/utils/builds'),
  USING_BUILD_BUCKET_DEV: false,
}));

describe('autorepairRequestsFromDuts', () => {
  it('handles empty', async () => {
    const request = autorepairRequestsFromDuts([], 'fake-session-id', false);

    expect(request).toEqual([]);
  });

  it('generates repair request', async () => {
    const request = autorepairRequestsFromDuts(
      [
        {
          name: 'test-dut',
          dutId: 'asset-tag',
          state: 'needs_manual_repair',
        },
      ],
      'fake-session-id',
      false,
    );

    expect(request.length).toEqual(1);

    expect(request[0].scheduleBuild?.requestId).toEqual(
      `fake-session-id-test-dut`,
    );
    expect(request[0].scheduleBuild?.priority).toEqual(TASK_PRIORITY);

    expect(request[0].scheduleBuild?.dimensions).toEqual([
      expect.objectContaining({ key: 'dut_id', value: 'asset-tag' }),
      expect.objectContaining({
        key: 'dut_state',
        value: 'needs_manual_repair',
      }),
    ]);
    expect(request[0].scheduleBuild?.tags).toEqual([
      {
        key: 'dut-name',
        value: `test-dut`,
      },
      {
        key: 'dut-id',
        value: `asset-tag`,
      },
      {
        key: 'admin-session',
        value: 'fake-session-id',
      },
      {
        key: 'task',
        value: REPAIR_TASK_NAME,
      },
      {
        key: 'client',
        value: 'fleet-console',
      },
      {
        key: 'version',
        value: 'prod',
      },
      {
        key: 'qs_account',
        value: 'unmanaged_p0',
      },
    ]);
  });

  it('generates deep repair request', async () => {
    const request = autorepairRequestsFromDuts(
      [
        {
          name: 'test-dut',
          dutId: 'asset-tag',
          state: 'needs_manual_repair',
        },
      ],
      'fake-session-id',
      true,
    );

    expect(request.length).toEqual(1);

    expect(request[0].scheduleBuild?.requestId).toEqual(
      `fake-session-id-test-dut`,
    );
    expect(request[0].scheduleBuild?.priority).toEqual(TASK_PRIORITY);

    expect(request[0].scheduleBuild?.dimensions).toEqual([
      expect.objectContaining({ key: 'dut_id', value: 'asset-tag' }),
      expect.objectContaining({
        key: 'dut_state',
        value: 'needs_manual_repair',
      }),
    ]);
    expect(request[0].scheduleBuild?.tags).toEqual([
      {
        key: 'dut-name',
        value: `test-dut`,
      },
      {
        key: 'dut-id',
        value: `asset-tag`,
      },
      {
        key: 'admin-session',
        value: 'fake-session-id',
      },
      {
        key: 'task',
        value: REPAIR_DEEP_TASK_NAME,
      },
      {
        key: 'client',
        value: 'fleet-console',
      },
      {
        key: 'version',
        value: 'prod',
      },
      {
        key: 'qs_account',
        value: 'unmanaged_p0',
      },
    ]);
  });

  it('includes targeted repair_requests in Buildbucket properties and Swarming tags', () => {
    const request = autorepairRequestsFromDuts(
      [
        {
          name: 'test-dut',
          dutId: 'asset-tag',
          state: 'needs_repair',
        },
      ],
      'fake-session-id',
      false,
      [],
      ['PROVISION', 'REIMAGE_BY_USBKEY', 'AUDIT_STORAGE'],
    );

    expect(request.length).toEqual(1);
    expect(request[0].scheduleBuild?.properties).toMatchObject({
      unit_name: 'test-dut',
      task_name: 'recovery',
      enable_recovery: true,
      update_inventory: true,
      repair_requests: ['PROVISION', 'REIMAGE_BY_USBKEY', 'AUDIT_STORAGE'],
    });
    expect(request[0].scheduleBuild?.tags).toEqual(
      expect.arrayContaining([
        { key: 'repair_request', value: 'PROVISION' },
        { key: 'repair_request', value: 'REIMAGE_BY_USBKEY' },
        { key: 'repair_request', value: 'AUDIT_STORAGE' },
      ]),
    );
  });

  it('formats shivas update dut-state before shivas repair when repairRequests are selected', () => {
    expect(
      formatShivasRepairCommand(['dut-1', 'dut-2'], {
        repairRequests: ['REIMAGE_BY_USBKEY', 'AUDIT_STORAGE'],
        latestRepair: true,
      }),
    ).toBe(
      [
        'shivas update dut-state -dut-name dut-1 -repair-request REIMAGE_BY_USBKEY,AUDIT_STORAGE',
        'shivas update dut-state -dut-name dut-2 -repair-request REIMAGE_BY_USBKEY,AUDIT_STORAGE',
        'shivas repair -latest dut-1 dut-2',
      ].join('\n'),
    );
  });

  it('serializes repairRequests in ScheduleAutorepairRequest.toJSON', () => {
    const req = createScheduleAutorepairRequest(
      ['dut-1'],
      [ScheduleAutorepairRequest_AutorepairFlag.LATEST],
      ['REIMAGE_BY_USBKEY', 'AUDIT_STORAGE'],
    );

    expect(req).toEqual({
      unitNames: ['dut-1'],
      flags: [ScheduleAutorepairRequest_AutorepairFlag.LATEST],
      repairRequests: ['REIMAGE_BY_USBKEY', 'AUDIT_STORAGE'],
    });
    expect(ScheduleAutorepairRequest.toJSON(req)).toEqual({
      unitNames: ['dut-1'],
      flags: ['LATEST'],
      repairRequests: ['REIMAGE_BY_USBKEY', 'AUDIT_STORAGE'],
    });
  });
});
