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
import { render, renderHook, screen, waitFor } from '@testing-library/react';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';

import {
  useAdminTaskPermission,
  usePermission,
} from './use_admin_task_permission';

jest.mock('@/fleet/hooks/prpc_clients', () => ({
  useFleetConsoleClient: jest.fn(),
}));

jest.mock('@/common/tools/logging', () => ({
  logging: {
    error: jest.fn(),
    warn: jest.fn(),
  },
}));

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
};

describe('usePermission', () => {
  const mockCheckPermission = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();

    const checkPermissionFn = jest
      .fn()
      .mockImplementation((req) => mockCheckPermission(req)) as jest.Mock & {
      query: jest.Mock;
    };
    checkPermissionFn.query = jest.fn().mockImplementation((req) => ({
      queryKey: ['CheckPermission', req.group],
      queryFn: () => mockCheckPermission(req),
    }));

    (useFleetConsoleClient as jest.Mock).mockReturnValue({
      CheckPermission: checkPermissionFn,
    });
  });

  it('initially returns null for hasPermission while loading', async () => {
    let resolvePromise: (val: unknown) => void = () => {};
    const promise = new Promise((resolve) => {
      resolvePromise = resolve;
    });
    mockCheckPermission.mockReturnValue(promise);

    const { result } = renderHook(() => usePermission('test-group'), {
      wrapper: createWrapper(),
    });

    expect(result.current.hasPermission).toBeNull();

    // Resolve the promise to avoid leaving pending tasks/warnings
    resolvePromise({ hasPermission: true });
    await waitFor(() => expect(result.current.hasPermission).toBe(true));
  });

  it('returns hasPermission boolean once resolved successfully', async () => {
    mockCheckPermission.mockResolvedValue({ hasPermission: true });

    const { result } = renderHook(() => usePermission('test-group'), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.hasPermission).toBe(true);
    });
  });

  it('returns false when CheckPermission rejects', async () => {
    mockCheckPermission.mockRejectedValue(
      new Error('Permission service unavailable'),
    );

    const { result } = renderHook(() => usePermission('test-group'), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.hasPermission).toBe(false);
    });
  });

  it('deduplicates multiple concurrent hook calls for the same group', async () => {
    mockCheckPermission.mockResolvedValue({ hasPermission: true });
    const Wrapper = createWrapper();

    function TestComponent() {
      const { hasPermission } = usePermission('test-group');
      return (
        <div>{hasPermission === null ? 'loading' : String(hasPermission)}</div>
      );
    }

    render(
      <Wrapper>
        <TestComponent />
        <TestComponent />
        <TestComponent />
        <TestComponent />
      </Wrapper>,
    );

    const els = await screen.findAllByText('true');
    expect(els).toHaveLength(4);
    expect(mockCheckPermission).toHaveBeenCalledTimes(1);
  });

  it('uses cached value and does not refetch on subsequent hook creations within 1 minute', async () => {
    mockCheckPermission.mockResolvedValue({ hasPermission: true });
    const wrapper = createWrapper();

    const { result: result1, unmount: unmount1 } = renderHook(
      () => usePermission('test-group'),
      {
        wrapper: wrapper,
      },
    );

    await waitFor(() => {
      expect(result1.current.hasPermission).toBe(true);
    });
    expect(mockCheckPermission).toHaveBeenCalledTimes(1);

    // Unmount first component
    unmount1();

    // Render hook again (within staleTime)
    const { result: result2 } = renderHook(() => usePermission('test-group'), {
      wrapper: wrapper,
    });

    await waitFor(() => {
      expect(result2.current.hasPermission).toBe(true);
    });
    // Should NOT have made a new call, since staleTime is 1 minute
    expect(mockCheckPermission).toHaveBeenCalledTimes(1);
  });

  it('fetchPermissions performs a direct client call and returns the response', async () => {
    mockCheckPermission.mockResolvedValue({ hasPermission: true });

    const { result } = renderHook(() => usePermission('test-group'), {
      wrapper: createWrapper(),
    });

    const resp = await result.current.fetchPermissions();
    expect(resp).toEqual({ hasPermission: true });
    expect(mockCheckPermission).toHaveBeenCalledWith({ group: 'test-group' });
  });

  describe('Shared Cache Isolation', () => {
    it('fetchPermissions does not pollute the cache or affect other hook instances on failure', async () => {
      mockCheckPermission.mockResolvedValue({ hasPermission: true });
      const wrapper = createWrapper();

      const { result: hookA } = renderHook(() => usePermission('test-group'), {
        wrapper,
      });
      const { result: hookB } = renderHook(() => usePermission('test-group'), {
        wrapper,
      });

      await waitFor(() => {
        expect(hookA.current.hasPermission).toBe(true);
        expect(hookB.current.hasPermission).toBe(true);
      });

      // fetchPermissions fails on hookA
      mockCheckPermission.mockRejectedValueOnce(new Error('Network error'));
      await expect(hookA.current.fetchPermissions()).rejects.toThrow(
        'Network error',
      );

      // Verify that hookB remains untouched (still has permission and is not in error state!)
      expect(hookB.current.hasPermission).toBe(true);
      expect(hookB.current.isError).toBe(false);
    });
  });

  describe('useAdminTaskPermission', () => {
    it('queries the correct policy group', async () => {
      mockCheckPermission.mockResolvedValue({ hasPermission: true });

      const { result } = renderHook(() => useAdminTaskPermission(), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(mockCheckPermission).toHaveBeenCalledWith({
          group: 'mdb/fleet-console-admin-tasks-policy',
        });
        expect(result.current.hasPermission).toBe(true);
      });
    });
  });
});
