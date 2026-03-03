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
import { renderHook, waitFor } from '@testing-library/react';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import {
  useAuthState,
  useGetAuthToken,
} from '@/common/components/auth_state_provider';

import { useUserProfile } from './use_user_profile';

jest.mock('@/common/components/auth_state_provider', () => ({
  useAuthState: jest.fn(),
  useGetAuthToken: jest.fn(),
  TokenType: { Access: 'access' },
}));

const createWrapper = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
};

describe('useUserProfile', () => {
  const mockGetAuthToken = jest.fn();

  beforeEach(() => {
    (useGetAuthToken as jest.Mock).mockReturnValue(mockGetAuthToken);
    global.fetch = jest.fn();
  });

  afterEach(() => {
    jest.resetAllMocks();
  });

  it('handles anonymous user', () => {
    (useAuthState as jest.Mock).mockReturnValue({
      identity: ANONYMOUS_IDENTITY,
    });

    const { result } = renderHook(() => useUserProfile(), {
      wrapper: createWrapper(),
    });

    expect(result.current.isAnonymous).toBe(true);
    expect(result.current.displayFirstName).toBe('');
    expect(result.current.email).toBeUndefined();
  });

  it('falls back to email prefix if auth API fails or given_name is missing', async () => {
    (useAuthState as jest.Mock).mockReturnValue({
      identity: 'user:jane.doe@example.com',
      email: 'jane.doe@example.com',
    });
    mockGetAuthToken.mockResolvedValue('fake-token');
    (global.fetch as jest.Mock).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({}),
    });

    const { result } = renderHook(() => useUserProfile(), {
      wrapper: createWrapper(),
    });

    expect(result.current.isAnonymous).toBe(false);
    expect(result.current.email).toBe('jane.doe@example.com');
    // initially synchronous fallback
    expect(result.current.displayFirstName).toBe('Jane.doe');

    await waitFor(() => {
      expect(result.current.query.isSuccess).toBe(true);
    });

    // still fallback
    expect(result.current.displayFirstName).toBe('Jane.doe');
  });

  it('uses given_name from userinfo API when available', async () => {
    (useAuthState as jest.Mock).mockReturnValue({
      identity: 'user:jane.doe@example.com',
      email: 'jane.doe@example.com',
      picture: 'fallback-pic.png',
    });
    mockGetAuthToken.mockResolvedValue('fake-token');
    (global.fetch as jest.Mock).mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({ given_name: 'Jane', picture: 'new-pic.png' }),
    });

    const { result } = renderHook(() => useUserProfile(), {
      wrapper: createWrapper(),
    });

    // initially synchronous fallback
    expect(result.current.displayFirstName).toBe('Jane.doe');

    await waitFor(() => {
      expect(result.current.query.isSuccess).toBe(true);
    });

    // switches to real name
    expect(result.current.displayFirstName).toBe('Jane');
    expect(result.current.picture).toBe('new-pic.png');
  });
});
