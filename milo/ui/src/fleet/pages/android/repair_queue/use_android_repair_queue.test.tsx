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

import { renderHook, waitFor } from '@testing-library/react';

import { FleetConsoleMockAPI } from '@/fleet/testing_tools/mock_api';
import {
  AndroidMachineType,
  AndroidWorkspace,
  ListAndroidRepairQueueRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { toAndroidWorkspaceEnum } from './android_workspace';
import { useAndroidRepairQueue } from './use_android_repair_queue';

describe('useAndroidRepairQueue', () => {
  beforeEach(() => {
    FleetConsoleMockAPI.resetFixtures();
    FleetConsoleMockAPI.enableBrowserInterceptor();
  });

  afterEach(() => {
    FleetConsoleMockAPI.disableBrowserInterceptor();
  });

  it('sends paging, filter and workspace to the RPC and exposes backend inProgressCount', async () => {
    // A function fixture returns the response verbatim (no in-memory
    // filter/paging simulation), so the hook's handling of the backend
    // paging fields and inProgressCount can be asserted directly.
    FleetConsoleMockAPI.setFixture('ListAndroidRepairQueue', () => ({
      repairQueueItems: [
        {
          deviceId: 'android-device-1',
          fcMachineType: AndroidMachineType.ANDROID_MACHINE_TYPE_DEVICE,
          hostGroups: ['group1'],
          runTarget: 'seahawk',
          state: 'MISSING',
          taskId: '101',
          claimedBy: '',
          priorityScore: '0',
          matchedRuleIds: [],
        },
      ],
      nextPageToken: 'next',
      totalSize: 42,
      inProgressCount: 7,
    }));

    const request = ListAndroidRepairQueueRequest.fromPartial({
      pageSize: 25,
      pageToken: 'page-2',
      filter: 'state = "MISSING"',
      workspace: toAndroidWorkspaceEnum('Pixel'),
    });

    const { result } = renderHook(() => useAndroidRepairQueue(request), {
      wrapper: FakeContextProvider,
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const calls = FleetConsoleMockAPI.getCalls('ListAndroidRepairQueue');
    expect(calls).toHaveLength(1);
    expect(calls[0].payload).toEqual(
      expect.objectContaining({
        pageSize: 25,
        pageToken: 'page-2',
        filter: 'state = "MISSING"',
        workspace: AndroidWorkspace[AndroidWorkspace.ANDROID_WORKSPACE_PIXEL],
      }),
    );

    expect(result.current.data?.repairQueueItems).toHaveLength(1);
    expect(result.current.data?.repairQueueItems[0].deviceId).toBe(
      'android-device-1',
    );
    expect(result.current.data?.nextPageToken).toBe('next');
    expect(result.current.data?.totalSize).toBe(42);
    // The count comes straight from the response, not from the page items.
    expect(result.current.inProgressCount).toBe(7);
  });

  it('defaults inProgressCount to 0 and surfaces RPC errors as isError', async () => {
    FleetConsoleMockAPI.setErrorFixture('ListAndroidRepairQueue', {
      message: 'workspace is required',
      grpcCode: 3,
    });

    const request = ListAndroidRepairQueueRequest.fromPartial({
      pageSize: 10,
      workspace: AndroidWorkspace.ANDROID_WORKSPACE_UNSPECIFIED,
    });

    const { result } = renderHook(() => useAndroidRepairQueue(request), {
      wrapper: FakeContextProvider,
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
    expect(result.current.inProgressCount).toBe(0);
  });
});
