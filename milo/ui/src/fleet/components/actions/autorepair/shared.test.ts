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
  autorepairRequestsFromDuts,
  REPAIR_DEEP_TASK_NAME,
  REPAIR_TASK_NAME,
  TASK_PRIORITY,
} from './shared';

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
      { key: 'dut_state', value: 'needs_manual_repair' },
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
      { key: 'dut_state', value: 'needs_manual_repair' },
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
});
