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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';

import { AuthState } from '@/common/api/auth_state';
import * as PrpcClientsModule from '@/fleet/hooks/prpc_clients';
import {
  AndroidMachineType,
  AndroidRepairQueueItem,
  ListAndroidRepairQueueResponse,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';

import { ANDROID_REPAIR_QUEUE_QUERY_KEY } from './use_android_repair_queue';
import {
  useClaimAndroidRepairTask,
  useUnclaimAndroidRepairTask,
} from './use_claim_android_repair_task';

const MOCK_ITEMS: readonly AndroidRepairQueueItem[] = [
  {
    deviceId: 'android-device-1',
    fcMachineType: AndroidMachineType.ANDROID_MACHINE_TYPE_DEVICE,
    hostGroups: ['group1'],
    runTarget: 'seahawk',
    state: 'MISSING',
    taskId: '101',
    claimedBy: '',
    claimedAt: undefined,
    hostGroupHealthPct: 0.85,
    runTargetHealthPct: 0.9,
    priorityScore: '100',
    labName: 'MTV-1',
    matchedRuleIds: [],
    etag: 'etag-101',
  },
  {
    deviceId: 'android-device-2',
    fcMachineType: AndroidMachineType.ANDROID_MACHINE_TYPE_DEVICE,
    hostGroups: ['group2'],
    runTarget: 'mustang',
    state: 'FAILED',
    taskId: '102',
    claimedBy: 'other_tech@google.com',
    claimedAt: '2026-09-16T10:00:00Z',
    hostGroupHealthPct: 0.5,
    runTargetHealthPct: 0.75,
    priorityScore: '80',
    labName: 'MTV-2',
    matchedRuleIds: [],
    etag: 'etag-102',
  },
];

describe('useClaimAndroidRepairTask and useUnclaimAndroidRepairTask', () => {
  let queryClient: QueryClient;
  let mockClaimAndroidRepairTask: jest.Mock;
  let mockUnclaimAndroidRepairTask: jest.Mock;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });

    mockClaimAndroidRepairTask = jest.fn();
    mockUnclaimAndroidRepairTask = jest.fn();

    const mockClient = {
      ClaimAndroidRepairTask: mockClaimAndroidRepairTask,
      UnclaimAndroidRepairTask: mockUnclaimAndroidRepairTask,
    } as unknown as ReturnType<typeof PrpcClientsModule.useFleetConsoleClient>;

    jest
      .spyOn(PrpcClientsModule, 'useFleetConsoleClient')
      .mockReturnValue(mockClient);
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  const createWrapper = (authState?: AuthState) => {
    const Wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>
        <FakeAuthStateProvider value={authState}>
          <>{children}</>
        </FakeAuthStateProvider>
      </QueryClientProvider>
    );
    Wrapper.displayName = 'TestWrapper';
    return Wrapper;
  };

  describe('useClaimAndroidRepairTask', () => {
    it('optimistically updates matching item on claim mutation', async () => {
      const initialData: ListAndroidRepairQueueResponse = {
        repairQueueItems: [...MOCK_ITEMS],
        nextPageToken: '',
        totalSize: 2,
        inProgressCount: 1,
      };

      const queryKey = [...ANDROID_REPAIR_QUEUE_QUERY_KEY, { pageSize: 100 }];
      queryClient.setQueryData(queryKey, initialData);

      let resolvePromise: (value: unknown) => void = () => {};
      mockClaimAndroidRepairTask.mockImplementation(
        () =>
          new Promise((resolve) => {
            resolvePromise = resolve;
          }),
      );

      const { result } = renderHook(() => useClaimAndroidRepairTask(), {
        wrapper: createWrapper(),
      });

      act(() => {
        result.current.mutate({ taskId: '101', etag: '' });
      });

      await waitFor(() => {
        const cachedData =
          queryClient.getQueryData<ListAndroidRepairQueueResponse>(queryKey);
        expect(cachedData?.repairQueueItems[0].claimedBy).toBe(
          'user@example.com',
        );
        expect(cachedData?.repairQueueItems[0].claimedAt).toBeDefined();
        expect(cachedData?.inProgressCount).toBe(2);
      });

      act(() => {
        resolvePromise({
          repairQueueItem: {
            ...MOCK_ITEMS[0],
            claimedBy: 'user@example.com',
          },
        });
      });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
    });

    it('rolls back optimistic update on error', async () => {
      const initialData: ListAndroidRepairQueueResponse = {
        repairQueueItems: [...MOCK_ITEMS],
        nextPageToken: '',
        totalSize: 2,
        inProgressCount: 1,
      };

      const queryKey = [...ANDROID_REPAIR_QUEUE_QUERY_KEY, { pageSize: 100 }];
      queryClient.setQueryData(queryKey, initialData);

      mockClaimAndroidRepairTask.mockRejectedValue(
        new Error('Network failure'),
      );

      const { result } = renderHook(() => useClaimAndroidRepairTask(), {
        wrapper: createWrapper(),
      });

      act(() => {
        result.current.mutate({ taskId: '101', etag: '' });
      });

      await waitFor(() => expect(result.current.isError).toBe(true));

      const cachedData =
        queryClient.getQueryData<ListAndroidRepairQueueResponse>(queryKey);
      expect(cachedData?.repairQueueItems[0].claimedBy).toBe('');
      expect(cachedData?.inProgressCount).toBe(1);
    });
  });

  describe('useUnclaimAndroidRepairTask', () => {
    it('optimistically clears claimedBy on unclaim mutation', async () => {
      const initialData: ListAndroidRepairQueueResponse = {
        repairQueueItems: [...MOCK_ITEMS],
        nextPageToken: '',
        totalSize: 2,
        inProgressCount: 1,
      };

      const queryKey = [...ANDROID_REPAIR_QUEUE_QUERY_KEY, { pageSize: 100 }];
      queryClient.setQueryData(queryKey, initialData);

      let resolvePromise: (value: unknown) => void = () => {};
      mockUnclaimAndroidRepairTask.mockImplementation(
        () =>
          new Promise((resolve) => {
            resolvePromise = resolve;
          }),
      );

      const { result } = renderHook(() => useUnclaimAndroidRepairTask(), {
        wrapper: createWrapper(),
      });

      act(() => {
        result.current.mutate({ taskId: '102', etag: '' });
      });

      await waitFor(() => {
        const cachedData =
          queryClient.getQueryData<ListAndroidRepairQueueResponse>(queryKey);
        expect(cachedData?.repairQueueItems[1].claimedBy).toBe('');
        expect(cachedData?.repairQueueItems[1].claimedAt).toBeUndefined();
        expect(cachedData?.inProgressCount).toBe(0);
      });

      act(() => {
        resolvePromise({
          repairQueueItem: {
            ...MOCK_ITEMS[1],
            claimedBy: '',
          },
        });
      });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
    });
  });
});
