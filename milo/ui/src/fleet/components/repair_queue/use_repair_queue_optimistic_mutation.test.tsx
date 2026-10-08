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

import { useRepairQueueOptimisticMutation } from './use_repair_queue_optimistic_mutation';

interface TestItem {
  readonly taskId: string;
  readonly claimedBy?: string;
  readonly label: string;
}

interface TestResponse {
  readonly repairQueueItems: readonly TestItem[];
  readonly inProgressCount?: number;
  readonly totalSize: number;
}

const TEST_QUERY_KEY = ['testRepairQueue'] as const;
const OTHER_QUERY_KEY = ['otherRepairQueue'] as const;

const INITIAL_DATA: TestResponse = {
  repairQueueItems: [
    { taskId: '1', claimedBy: '', label: 'a' },
    { taskId: '2', claimedBy: 'someone@google.com', label: 'b' },
  ],
  inProgressCount: 1,
  totalSize: 2,
};

describe('useRepairQueueOptimisticMutation', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
  });

  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );

  const renderMutation = (
    mutationFn: (req: { taskId: string }) => Promise<unknown>,
    claimedBy: string,
  ) =>
    renderHook(
      () =>
        useRepairQueueOptimisticMutation<unknown, { taskId: string }, TestItem>(
          {
            queryKey: TEST_QUERY_KEY,
            mutationFn,
            updateItem: (item) => ({ ...item, claimedBy }),
          },
        ),
      { wrapper },
    );

  it('optimistically updates only queries under the given key', async () => {
    const queryKey = [...TEST_QUERY_KEY, { pageSize: 100 }];
    const otherKey = [...OTHER_QUERY_KEY, { pageSize: 100 }];
    queryClient.setQueryData(queryKey, INITIAL_DATA);
    queryClient.setQueryData(otherKey, INITIAL_DATA);

    const { result } = renderMutation(
      () => new Promise(() => {}),
      'me@google.com',
    );

    act(() => {
      result.current.mutate({ taskId: '1' });
    });

    await waitFor(() => {
      const cached = queryClient.getQueryData<TestResponse>(queryKey);
      expect(cached?.repairQueueItems[0]).toEqual({
        taskId: '1',
        claimedBy: 'me@google.com',
        label: 'a',
      });
      expect(cached?.inProgressCount).toBe(2);
      // Fields that the hook does not know about are preserved.
      expect(cached?.totalSize).toBe(2);
    });
    expect(queryClient.getQueryData<TestResponse>(otherKey)).toBe(INITIAL_DATA);
  });

  it('decrements inProgressCount when an item is unclaimed', async () => {
    const queryKey = [...TEST_QUERY_KEY, { pageSize: 100 }];
    queryClient.setQueryData(queryKey, INITIAL_DATA);

    const { result } = renderMutation(() => new Promise(() => {}), '');

    act(() => {
      result.current.mutate({ taskId: '2' });
    });

    await waitFor(() => {
      const cached = queryClient.getQueryData<TestResponse>(queryKey);
      expect(cached?.repairQueueItems[1].claimedBy).toBe('');
      expect(cached?.inProgressCount).toBe(0);
    });
  });

  it('optimistically updates inProgressCount across all cached queries under queryKey', async () => {
    const headerKey = [...TEST_QUERY_KEY, { pageSize: 1 }];
    const tableKey = [...TEST_QUERY_KEY, { pageSize: 100 }];
    const headerData: TestResponse = {
      repairQueueItems: [{ taskId: '1', claimedBy: '', label: 'a' }],
      inProgressCount: 1,
      totalSize: 3,
    };
    const tableData: TestResponse = {
      repairQueueItems: [
        { taskId: '1', claimedBy: '', label: 'a' },
        { taskId: '2', claimedBy: 'someone@google.com', label: 'b' },
        { taskId: '3', claimedBy: '', label: 'c' },
      ],
      inProgressCount: 1,
      totalSize: 3,
    };
    queryClient.setQueryData(headerKey, headerData);
    queryClient.setQueryData(tableKey, tableData);

    const { result: claimMutation } = renderMutation(
      () => new Promise(() => {}),
      'me@google.com',
    );

    act(() => {
      claimMutation.current.mutate({ taskId: '3' });
    });

    await waitFor(() => {
      const cachedHeader = queryClient.getQueryData<TestResponse>(headerKey);
      const cachedTable = queryClient.getQueryData<TestResponse>(tableKey);
      expect(cachedHeader?.inProgressCount).toBe(2);
      expect(cachedHeader?.repairQueueItems).toEqual(
        headerData.repairQueueItems,
      );
      expect(cachedTable?.inProgressCount).toBe(2);
      expect(cachedTable?.repairQueueItems[2].claimedBy).toBe('me@google.com');
    });

    const { result: unclaimMutation } = renderMutation(
      () => new Promise(() => {}),
      '',
    );

    act(() => {
      unclaimMutation.current.mutate({ taskId: '2' });
    });

    await waitFor(() => {
      const cachedHeader = queryClient.getQueryData<TestResponse>(headerKey);
      const cachedTable = queryClient.getQueryData<TestResponse>(tableKey);
      expect(cachedHeader?.inProgressCount).toBe(1);
      expect(cachedTable?.inProgressCount).toBe(1);
      expect(cachedTable?.repairQueueItems[1].claimedBy).toBe('');
    });
  });

  it('rolls back on error and invalidates the given key on settled', async () => {
    const queryKey = [...TEST_QUERY_KEY, { pageSize: 100 }];
    queryClient.setQueryData(queryKey, INITIAL_DATA);
    const invalidateSpy = jest.spyOn(queryClient, 'invalidateQueries');

    const { result } = renderMutation(
      () => Promise.reject(new Error('RPC Failed')),
      'me@google.com',
    );

    act(() => {
      result.current.mutate({ taskId: '1' });
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(queryClient.getQueryData<TestResponse>(queryKey)).toEqual(
      INITIAL_DATA,
    );
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: TEST_QUERY_KEY });
  });
});
