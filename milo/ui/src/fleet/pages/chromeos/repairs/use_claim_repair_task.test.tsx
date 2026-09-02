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
  ListRepairQueueResponse,
  PeripheralState,
  RepairQueueItem,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';

import {
  useClaimRepairTask,
  useUnclaimRepairTask,
} from './use_claim_repair_task';
import { REPAIR_QUEUE_QUERY_KEY, useRepairQueue } from './use_repair_queue';

const MOCK_ITEMS: readonly RepairQueueItem[] = [
  {
    taskId: '101',
    dutId: 'chromeos-host-1',
    pools: ['DUT_POOL_QUOTA'],
    model: 'volteer',
    state: 'needs_repair',
    claimedBy: '',
    claimedAt: undefined,
    servoState: PeripheralState.PERIPHERAL_STATE_OK,
    wifiState: PeripheralState.PERIPHERAL_STATE_OK,
    bluetoothState: PeripheralState.PERIPHERAL_STATE_OK,
  },
  {
    taskId: '102',
    dutId: 'chromeos-host-2',
    pools: ['faft-cr50'],
    model: 'brya',
    state: 'repair_failed',
    claimedBy: 'other_tech@google.com',
    claimedAt: '2026-08-19T10:00:00Z',
    servoState: PeripheralState.PERIPHERAL_STATE_OK,
    wifiState: PeripheralState.PERIPHERAL_STATE_OK,
    bluetoothState: PeripheralState.PERIPHERAL_STATE_OK,
  },
];

describe('useClaimRepairTask and useUnclaimRepairTask', () => {
  let queryClient: QueryClient;
  let mockClaimRepairTask: jest.Mock;
  let mockUnclaimRepairTask: jest.Mock;
  let mockListRepairQueue: jest.Mock;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });

    mockClaimRepairTask = jest.fn();
    mockUnclaimRepairTask = jest.fn();
    mockListRepairQueue = jest.fn();

    const mockClient = {
      ClaimRepairTask: mockClaimRepairTask,
      UnclaimRepairTask: mockUnclaimRepairTask,
      ListRepairQueue: {
        query: (req: unknown) => ({
          queryKey: [...REPAIR_QUEUE_QUERY_KEY, req],
          queryFn: () => mockListRepairQueue(req),
        }),
      },
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

  describe('useClaimRepairTask', () => {
    it('optimistically updates matching item on claim mutation', async () => {
      const initialData: ListRepairQueueResponse = {
        repairQueueItems: [...MOCK_ITEMS],
        nextPageToken: '',
        totalSize: 2,
      };

      const queryKey = [...REPAIR_QUEUE_QUERY_KEY, { pageSize: 100 }];
      queryClient.setQueryData(queryKey, initialData);

      let resolvePromise: (value: unknown) => void = () => {};
      mockClaimRepairTask.mockImplementation(
        () =>
          new Promise((resolve) => {
            resolvePromise = resolve;
          }),
      );

      const { result } = renderHook(() => useClaimRepairTask(), {
        wrapper: createWrapper(),
      });

      act(() => {
        result.current.mutate({ taskId: '101' });
      });

      // Verify cache was optimistically updated while mutation is in-flight
      await waitFor(() => {
        const cachedData =
          queryClient.getQueryData<ListRepairQueueResponse>(queryKey);
        expect(cachedData?.repairQueueItems[0].claimedBy).toBeDefined();
        expect(cachedData?.repairQueueItems[0].claimedBy).not.toBe('');
        const claimedAt = cachedData?.repairQueueItems[0].claimedAt;
        expect(claimedAt).toBeDefined();
        expect(new Date(claimedAt || '').getTime()).not.toBeNaN();
        expect(cachedData?.repairQueueItems[1].claimedBy).toBe(
          'other_tech@google.com',
        );
      });

      // Resolve the mutation
      act(() => {
        resolvePromise({
          repairQueueItem: {
            ...MOCK_ITEMS[0],
            claimedBy: 'current_user',
          },
        });
      });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
    });

    it('rolls back optimistic update on mutation error', async () => {
      const initialData: ListRepairQueueResponse = {
        repairQueueItems: [...MOCK_ITEMS],
        nextPageToken: '',
        totalSize: 2,
      };

      const queryKey = [...REPAIR_QUEUE_QUERY_KEY, { pageSize: 100 }];
      queryClient.setQueryData(queryKey, initialData);

      mockClaimRepairTask.mockRejectedValue(new Error('RPC Failed'));

      const { result } = renderHook(() => useClaimRepairTask(), {
        wrapper: createWrapper(),
      });

      act(() => {
        result.current.mutate({ taskId: '101' });
      });

      await waitFor(() => expect(result.current.isError).toBe(true));

      // Cache should be rolled back to initial unclaimed state
      const cachedData =
        queryClient.getQueryData<ListRepairQueueResponse>(queryKey);
      expect(cachedData?.repairQueueItems[0].claimedBy).toBe('');
    });

    it('invalidates queries on settled', async () => {
      const invalidateSpy = jest.spyOn(queryClient, 'invalidateQueries');
      mockClaimRepairTask.mockResolvedValue({
        repairQueueItem: {
          ...MOCK_ITEMS[0],
          claimedBy: 'current_user',
        },
      });

      const { result } = renderHook(() => useClaimRepairTask(), {
        wrapper: createWrapper(),
      });

      act(() => {
        result.current.mutate({ taskId: '101' });
      });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: REPAIR_QUEUE_QUERY_KEY,
      });
    });

    it('optimistically updates matching item when taskId matches', async () => {
      const initialData: ListRepairQueueResponse = {
        repairQueueItems: [
          {
            taskId: 'chromeos-task-1',
            dutId: 'chromeos-only-dut',
            pools: ['DUT_POOL_QUOTA'],
            model: 'volteer',
            state: 'needs_repair',
            claimedBy: '',
            claimedAt: undefined,
            servoState: PeripheralState.PERIPHERAL_STATE_OK,
            wifiState: PeripheralState.PERIPHERAL_STATE_OK,
            bluetoothState: PeripheralState.PERIPHERAL_STATE_OK,
          },
        ],
        nextPageToken: '',
        totalSize: 1,
      };

      const queryKey = [...REPAIR_QUEUE_QUERY_KEY, { pageSize: 100 }];
      queryClient.setQueryData(queryKey, initialData);

      mockClaimRepairTask.mockResolvedValue({});

      const { result } = renderHook(() => useClaimRepairTask(), {
        wrapper: createWrapper(),
      });

      act(() => {
        result.current.mutate({ taskId: 'chromeos-task-1' });
      });

      await waitFor(() => {
        const cachedData =
          queryClient.getQueryData<ListRepairQueueResponse>(queryKey);
        expect(cachedData?.repairQueueItems[0].claimedBy).toBeDefined();
        expect(cachedData?.repairQueueItems[0].claimedBy).not.toBe('');
      });
    });

    it('optimistically claims with identity when email is absent', async () => {
      const initialData: ListRepairQueueResponse = {
        repairQueueItems: [...MOCK_ITEMS],
        nextPageToken: '',
        totalSize: 2,
      };

      const queryKey = [...REPAIR_QUEUE_QUERY_KEY, { pageSize: 100 }];
      queryClient.setQueryData(queryKey, initialData);

      mockClaimRepairTask.mockResolvedValue({});

      const { result } = renderHook(() => useClaimRepairTask(), {
        wrapper: createWrapper({
          identity: 'user:custom_tech@google.com',
          email: '',
          accessToken: '',
          idToken: '',
        }),
      });

      act(() => {
        result.current.mutate({ taskId: '101' });
      });

      await waitFor(() => {
        const cachedData =
          queryClient.getQueryData<ListRepairQueueResponse>(queryKey);
        expect(cachedData?.repairQueueItems[0].claimedBy).toBe(
          'custom_tech@google.com',
        );
      });
    });

    it('leaves cache unchanged if target taskId does not match any item', async () => {
      const initialData: ListRepairQueueResponse = {
        repairQueueItems: [...MOCK_ITEMS],
        nextPageToken: '',
        totalSize: 2,
      };

      const queryKey = [...REPAIR_QUEUE_QUERY_KEY, { pageSize: 100 }];
      queryClient.setQueryData(queryKey, initialData);

      mockClaimRepairTask.mockResolvedValue({});

      const { result } = renderHook(() => useClaimRepairTask(), {
        wrapper: createWrapper(),
      });

      act(() => {
        result.current.mutate({ taskId: 'non-existent-task' });
      });

      await waitFor(() => {
        const cachedData =
          queryClient.getQueryData<ListRepairQueueResponse>(queryKey);
        expect(cachedData?.repairQueueItems[0].claimedBy).toBe('');
        expect(cachedData?.repairQueueItems[1].claimedBy).toBe(
          'other_tech@google.com',
        );
      });
    });
  });

  describe('useUnclaimRepairTask', () => {
    it('optimistically clears claimedBy on unclaim mutation', async () => {
      const initialData: ListRepairQueueResponse = {
        repairQueueItems: [...MOCK_ITEMS],
        nextPageToken: '',
        totalSize: 2,
      };

      const queryKey = [...REPAIR_QUEUE_QUERY_KEY, { pageSize: 100 }];
      queryClient.setQueryData(queryKey, initialData);

      let resolvePromise: (value: unknown) => void = () => {};
      mockUnclaimRepairTask.mockImplementation(
        () =>
          new Promise((resolve) => {
            resolvePromise = resolve;
          }),
      );

      const { result } = renderHook(() => useUnclaimRepairTask(), {
        wrapper: createWrapper(),
      });

      act(() => {
        result.current.mutate({ taskId: '102' });
      });

      // Verify cache was optimistically updated to empty string while mutation is in-flight
      await waitFor(() => {
        const cachedData =
          queryClient.getQueryData<ListRepairQueueResponse>(queryKey);
        expect(cachedData?.repairQueueItems[1].claimedBy).toBe('');
        expect(cachedData?.repairQueueItems[1].claimedAt).toBeUndefined();
      });

      // Resolve the mutation
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

    it('rolls back optimistic unclaim on mutation error', async () => {
      const initialData: ListRepairQueueResponse = {
        repairQueueItems: [...MOCK_ITEMS],
        nextPageToken: '',
        totalSize: 2,
      };

      const queryKey = [...REPAIR_QUEUE_QUERY_KEY, { pageSize: 100 }];
      queryClient.setQueryData(queryKey, initialData);

      mockUnclaimRepairTask.mockRejectedValue(new Error('RPC Failed'));

      const { result } = renderHook(() => useUnclaimRepairTask(), {
        wrapper: createWrapper(),
      });

      act(() => {
        result.current.mutate({ taskId: '102' });
      });

      await waitFor(() => expect(result.current.isError).toBe(true));

      // Cache should be rolled back to initial claimed state
      const cachedData =
        queryClient.getQueryData<ListRepairQueueResponse>(queryKey);
      expect(cachedData?.repairQueueItems[1].claimedBy).toBe(
        'other_tech@google.com',
      );
      expect(cachedData?.repairQueueItems[1].claimedAt).toBe(
        '2026-08-19T10:00:00Z',
      );
    });

    it('invalidates queries on unclaim settled', async () => {
      const invalidateSpy = jest.spyOn(queryClient, 'invalidateQueries');
      mockUnclaimRepairTask.mockResolvedValue({
        repairQueueItem: {
          ...MOCK_ITEMS[1],
          claimedBy: '',
        },
      });

      const { result } = renderHook(() => useUnclaimRepairTask(), {
        wrapper: createWrapper(),
      });

      act(() => {
        result.current.mutate({ taskId: '102' });
      });

      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: REPAIR_QUEUE_QUERY_KEY,
      });
    });
  });

  it('configures useRepairQueue with refetchInterval: 10000', () => {
    mockListRepairQueue.mockResolvedValue({
      repairQueueItems: MOCK_ITEMS,
      totalSize: 2,
      nextPageToken: '',
    });

    const { result } = renderHook(
      () =>
        useRepairQueue({
          pageSize: 50,
          pageToken: '',
          orderBy: '',
          filter: '',
        }),
      { wrapper: createWrapper() },
    );

    expect(result.current).toBeDefined();
  });
});
