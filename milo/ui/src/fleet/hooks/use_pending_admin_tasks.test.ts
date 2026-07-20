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

import { renderHook } from '@testing-library/react';

import { useAuthState } from '@/common/components/auth_state_provider';
import { useTasks } from '@/fleet/hooks/swarming_hooks';

import {
  useAdminTaskTags,
  usePendingAdminTasksCount,
} from './use_pending_admin_tasks';

jest.mock('@/common/components/auth_state_provider');
jest.mock('@/fleet/hooks/swarming_hooks');
jest.mock('@/swarming/hooks/prpc_clients', () => ({
  useTasksClient: jest.fn(),
}));

describe('use_pending_admin_tasks hooks', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('useAdminTaskTags', () => {
    it('returns client_user tag when email is present', () => {
      (useAuthState as jest.Mock).mockReturnValue({ email: 'user@google.com' });
      const { result } = renderHook(() => useAdminTaskTags());
      expect(result.current).toEqual(['client_user:user@google.com']);
    });

    it('returns empty array when email is undefined', () => {
      (useAuthState as jest.Mock).mockReturnValue({ email: undefined });
      const { result } = renderHook(() => useAdminTaskTags());
      expect(result.current).toEqual([]);
    });
  });

  describe('usePendingAdminTasksCount', () => {
    it('returns task count when logged in', () => {
      (useAuthState as jest.Mock).mockReturnValue({ email: 'user@google.com' });
      (useTasks as jest.Mock).mockReturnValue({
        tasks: [{ taskId: '1' }, { taskId: '2' }],
        isLoading: false,
      });

      const { result } = renderHook(() => usePendingAdminTasksCount());
      expect(result.current).toBe(2);
    });

    it('returns 0 when unauthenticated', () => {
      (useAuthState as jest.Mock).mockReturnValue({ email: undefined });
      (useTasks as jest.Mock).mockReturnValue({
        tasks: undefined,
        isLoading: false,
      });

      const { result } = renderHook(() => usePendingAdminTasksCount());
      expect(result.current).toBe(0);
    });

    it('disables query when enabled is false', () => {
      (useAuthState as jest.Mock).mockReturnValue({ email: 'user@google.com' });
      (useTasks as jest.Mock).mockReturnValue({
        tasks: undefined,
        isLoading: false,
      });

      const { result } = renderHook(() =>
        usePendingAdminTasksCount({ enabled: false }),
      );
      expect(result.current).toBe(0);
      expect(useTasks).toHaveBeenCalledWith(
        expect.anything(),
        expect.objectContaining({ enabled: false }),
      );
    });
  });
});
