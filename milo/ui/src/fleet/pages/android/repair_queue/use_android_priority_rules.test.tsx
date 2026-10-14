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

import * as PrpcClientsModule from '@/fleet/hooks/prpc_clients';
import {
  AndroidPriorityRule,
  AndroidWorkspace,
  ListAndroidPriorityRulesRequest,
  ListAndroidPriorityRulesResponse,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  getAndroidPriorityRulesQueryKey,
  useAndroidPriorityRules,
  useCreateAndroidPriorityRule,
  useDeleteAndroidPriorityRule,
  useUpdateAndroidPriorityRule,
} from './use_android_priority_rules';

const MOCK_PIXEL_RULES: readonly AndroidPriorityRule[] = [
  {
    id: '1',
    expressionAip160: 'run_target = "seahawk"',
    weight: '50',
    workspace: AndroidWorkspace.ANDROID_WORKSPACE_PIXEL,
  },
  {
    id: '2',
    expressionAip160: 'state = "MISSING"',
    weight: '20',
    workspace: AndroidWorkspace.ANDROID_WORKSPACE_PIXEL,
  },
];

const MOCK_ANDROID_RULES: readonly AndroidPriorityRule[] = [
  {
    id: '3',
    expressionAip160: 'state = "MISSING"',
    weight: '100',
    workspace: AndroidWorkspace.ANDROID_WORKSPACE_ANDROID,
  },
];

describe('use_android_priority_rules', () => {
  let queryClient: QueryClient;
  let mockListRules: jest.Mock;
  let mockCreateRule: jest.Mock;
  let mockUpdateRule: jest.Mock;
  let mockDeleteRule: jest.Mock;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });

    mockListRules = jest.fn();
    mockCreateRule = jest.fn();
    mockUpdateRule = jest.fn();
    mockDeleteRule = jest.fn();

    const mockClient = {
      ListAndroidPriorityRules: {
        query: (req: ListAndroidPriorityRulesRequest) => ({
          queryKey: getAndroidPriorityRulesQueryKey(
            req.workspace === AndroidWorkspace.ANDROID_WORKSPACE_PIXEL
              ? 'Pixel'
              : 'Android',
          ),
          queryFn: () => mockListRules(req),
        }),
      },
      CreateAndroidPriorityRule: mockCreateRule,
      UpdateAndroidPriorityRule: mockUpdateRule,
      DeleteAndroidPriorityRule: mockDeleteRule,
    } as unknown as ReturnType<typeof PrpcClientsModule.useFleetConsoleClient>;

    jest
      .spyOn(PrpcClientsModule, 'useFleetConsoleClient')
      .mockReturnValue(mockClient);
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );

  const getCachedRules = (workspace: 'Android' | 'Pixel') =>
    queryClient.getQueryData<ListAndroidPriorityRulesResponse>(
      getAndroidPriorityRulesQueryKey(workspace),
    )?.priorityRules;

  it('generates independent query keys for Pixel and Android workspaces', () => {
    expect(getAndroidPriorityRulesQueryKey('Pixel')).toEqual([
      'ListAndroidPriorityRules',
      'Pixel',
    ]);
    expect(getAndroidPriorityRulesQueryKey('Android')).toEqual([
      'ListAndroidPriorityRules',
      'Android',
    ]);
  });

  it('fetches rules scoped to the Pixel workspace', async () => {
    mockListRules.mockResolvedValue({
      priorityRules: MOCK_PIXEL_RULES,
    });

    const { result } = renderHook(() => useAndroidPriorityRules('Pixel'), {
      wrapper,
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(mockListRules).toHaveBeenCalledWith({
      workspace: AndroidWorkspace.ANDROID_WORKSPACE_PIXEL,
    });
    expect(result.current.rules).toEqual(MOCK_PIXEL_RULES);
  });

  it('fetches rules scoped to the Android workspace', async () => {
    mockListRules.mockResolvedValue({
      priorityRules: MOCK_ANDROID_RULES,
    });

    const { result } = renderHook(() => useAndroidPriorityRules('Android'), {
      wrapper,
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(mockListRules).toHaveBeenCalledWith({
      workspace: AndroidWorkspace.ANDROID_WORKSPACE_ANDROID,
    });
    expect(result.current.rules).toEqual(MOCK_ANDROID_RULES);
  });

  describe('useCreateAndroidPriorityRule', () => {
    it('sets the workspace on the rule only (no request-level workspace)', async () => {
      mockCreateRule.mockResolvedValue({
        priorityRule: MOCK_PIXEL_RULES[0],
      });

      const { result } = renderHook(
        () => useCreateAndroidPriorityRule('Pixel'),
        { wrapper },
      );

      await act(async () => {
        await result.current.mutateAsync({
          priorityRule: {
            id: '0',
            expressionAip160: 'run_target = "seahawk"',
            weight: '50',
          },
        });
      });

      expect(mockCreateRule).toHaveBeenCalledTimes(1);
      expect(mockCreateRule).toHaveBeenCalledWith({
        priorityRule: {
          id: '0',
          expressionAip160: 'run_target = "seahawk"',
          weight: '50',
          workspace: AndroidWorkspace.ANDROID_WORKSPACE_PIXEL,
        },
      });
      expect(mockCreateRule.mock.calls[0][0]).not.toHaveProperty('workspace');
    });

    it('leaves the cached rules untouched and surfaces the error on failure', async () => {
      queryClient.setQueryData(getAndroidPriorityRulesQueryKey('Pixel'), {
        priorityRules: MOCK_PIXEL_RULES,
      });
      mockCreateRule.mockRejectedValue(new Error('create failed'));

      const { result } = renderHook(
        () => useCreateAndroidPriorityRule('Pixel'),
        { wrapper },
      );

      await act(async () => {
        await expect(
          result.current.mutateAsync({
            priorityRule: { id: '0', expressionAip160: '', weight: '1' },
          }),
        ).rejects.toThrow('create failed');
      });

      await waitFor(() => expect(result.current.isError).toBe(true));
      expect(getCachedRules('Pixel')).toEqual(MOCK_PIXEL_RULES);
    });
  });

  describe('useUpdateAndroidPriorityRule', () => {
    it('scopes the request to the workspace and optimistically updates the rule', async () => {
      queryClient.setQueryData(getAndroidPriorityRulesQueryKey('Pixel'), {
        priorityRules: MOCK_PIXEL_RULES,
      });

      let resolveUpdate: (value: unknown) => void = () => {};
      mockUpdateRule.mockImplementation(
        () =>
          new Promise((resolve) => {
            resolveUpdate = resolve;
          }),
      );

      const { result } = renderHook(
        () => useUpdateAndroidPriorityRule('Pixel'),
        { wrapper },
      );

      act(() => {
        result.current.mutate({
          id: '1',
          expressionAip160: 'run_target = "seahawk"',
          weight: '80',
        });
      });

      await waitFor(() =>
        expect(getCachedRules('Pixel')?.[0].weight).toBe('80'),
      );
      expect(getCachedRules('Pixel')?.[1]).toEqual(MOCK_PIXEL_RULES[1]);
      expect(mockUpdateRule).toHaveBeenCalledWith({
        id: '1',
        expressionAip160: 'run_target = "seahawk"',
        weight: '80',
        workspace: AndroidWorkspace.ANDROID_WORKSPACE_PIXEL,
      });

      act(() => {
        resolveUpdate({
          priorityRule: { ...MOCK_PIXEL_RULES[0], weight: '80' },
        });
      });
      await waitFor(() => expect(result.current.isSuccess).toBe(true));
    });

    it('rolls back the optimistic update on error', async () => {
      queryClient.setQueryData(getAndroidPriorityRulesQueryKey('Pixel'), {
        priorityRules: MOCK_PIXEL_RULES,
      });
      mockUpdateRule.mockRejectedValue(new Error('update failed'));

      const { result } = renderHook(
        () => useUpdateAndroidPriorityRule('Pixel'),
        { wrapper },
      );

      act(() => {
        result.current.mutate({
          id: '1',
          expressionAip160: 'run_target = "seahawk"',
          weight: '80',
        });
      });

      await waitFor(() => expect(result.current.isError).toBe(true));
      expect(getCachedRules('Pixel')).toEqual(MOCK_PIXEL_RULES);
    });
  });

  describe('useDeleteAndroidPriorityRule', () => {
    it('scopes the request to the workspace and optimistically removes the rule', async () => {
      queryClient.setQueryData(getAndroidPriorityRulesQueryKey('Pixel'), {
        priorityRules: MOCK_PIXEL_RULES,
      });

      let resolveDelete: (value: unknown) => void = () => {};
      mockDeleteRule.mockImplementation(
        () =>
          new Promise((resolve) => {
            resolveDelete = resolve;
          }),
      );

      const { result } = renderHook(
        () => useDeleteAndroidPriorityRule('Pixel'),
        { wrapper },
      );

      act(() => {
        result.current.mutate({ id: '1' });
      });

      await waitFor(() =>
        expect(getCachedRules('Pixel')).toEqual([MOCK_PIXEL_RULES[1]]),
      );
      expect(mockDeleteRule).toHaveBeenCalledWith({
        id: '1',
        workspace: AndroidWorkspace.ANDROID_WORKSPACE_PIXEL,
      });

      act(() => {
        resolveDelete({});
      });
      await waitFor(() => expect(result.current.isSuccess).toBe(true));
    });

    it('rolls back the optimistic removal on error', async () => {
      queryClient.setQueryData(getAndroidPriorityRulesQueryKey('Pixel'), {
        priorityRules: MOCK_PIXEL_RULES,
      });
      mockDeleteRule.mockRejectedValue(new Error('delete failed'));

      const { result } = renderHook(
        () => useDeleteAndroidPriorityRule('Pixel'),
        { wrapper },
      );

      act(() => {
        result.current.mutate({ id: '1' });
      });

      await waitFor(() => expect(result.current.isError).toBe(true));
      expect(getCachedRules('Pixel')).toEqual(MOCK_PIXEL_RULES);
    });
  });
});
