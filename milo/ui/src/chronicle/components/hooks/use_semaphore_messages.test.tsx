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
import { ReactNode } from 'react';

import { WorkPlan } from '@/proto/turboci/graph/orchestrator/v1/workplan.pb';
import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ChronicleContext, ChronicleContextType } from '../context';

import {
  SEMAPHORE_POLL_INTERVAL_MS,
  useSemaphoreMessages,
} from './use_semaphore_messages';

const SEMAPHORE_HOST = 'semaphore.example.com';
const SEMAPHORE_API_KEY = 'test-semaphore-api-key';

function jsonResponse(body: unknown): Response {
  return { ok: true, status: 200, json: async () => body } as Response;
}

describe('useSemaphoreMessages', () => {
  const originalFetch = global.fetch;
  const originalSettings = SETTINGS;
  let mockFetch: jest.MockedFunction<typeof fetch>;

  beforeEach(() => {
    mockFetch = jest.fn();
    global.fetch = mockFetch;
    (self as unknown as { SETTINGS: typeof SETTINGS }).SETTINGS = Object.freeze(
      {
        ...originalSettings,
        chronicle: {
          semaphoreHost: SEMAPHORE_HOST,
          semaphoreApiKey: SEMAPHORE_API_KEY,
        },
      },
    );
  });

  afterEach(() => {
    jest.useRealTimers();
    global.fetch = originalFetch;
    (self as unknown as { SETTINGS: typeof SETTINGS }).SETTINGS =
      originalSettings;
  });

  function createWrapper(email = 'tester@google.com', contextRealm?: string) {
    return function Wrapper({ children }: { children: ReactNode }) {
      const inner = contextRealm ? (
        <ChronicleContext.Provider
          value={
            {
              graph: { realm: contextRealm } as WorkPlan,
            } as ChronicleContextType
          }
        >
          {children}
        </ChronicleContext.Provider>
      ) : (
        children
      );
      return (
        <FakeContextProvider>
          <FakeAuthStateProvider value={{ identity: `user:${email}`, email }}>
            <>{inner}</>
          </FakeAuthStateProvider>
        </FakeContextProvider>
      );
    };
  }

  it('does not query Semaphore for non-Googlers', () => {
    const { result } = renderHook(
      () => useSemaphoreMessages('android:presubmit'),
      { wrapper: createWrapper('user@example.com') },
    );

    expect(mockFetch).not.toHaveBeenCalled();
    expect(result.current).toEqual([]);
  });

  it('does not query Semaphore for non-Android or undefined realms', () => {
    const { result, rerender } = renderHook(
      ({ realm }: { realm?: string }) => useSemaphoreMessages(realm),
      {
        initialProps: { realm: 'chromium:ci' as string | undefined },
        wrapper: createWrapper(),
      },
    );

    expect(mockFetch).not.toHaveBeenCalled();
    expect(result.current).toEqual([]);

    rerender({ realm: undefined });
    expect(mockFetch).not.toHaveBeenCalled();
    expect(result.current).toEqual([]);
  });

  it('does not query Semaphore when Chronicle settings are omitted or empty', () => {
    (self as unknown as { SETTINGS: typeof SETTINGS }).SETTINGS = Object.freeze(
      {
        ...originalSettings,
        chronicle: undefined,
      },
    );

    const { result, unmount } = renderHook(
      () => useSemaphoreMessages('android:presubmit'),
      { wrapper: createWrapper() },
    );
    expect(mockFetch).not.toHaveBeenCalled();
    expect(result.current).toEqual([]);
    unmount();

    (self as unknown as { SETTINGS: typeof SETTINGS }).SETTINGS = Object.freeze(
      {
        ...originalSettings,
        chronicle: { semaphoreHost: '', semaphoreApiKey: '' },
      },
    );

    const { result: emptyResult } = renderHook(
      () => useSemaphoreMessages('android:presubmit'),
      { wrapper: createWrapper() },
    );
    expect(mockFetch).not.toHaveBeenCalled();
    expect(emptyResult.current).toEqual([]);
  });

  it('reads realm from ChronicleContext when propRealm is omitted', async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({
        messages: [
          {
            name: 'messages/1',
            longMessage: 'Android outage',
            urgency: 'FAIL',
          },
        ],
      }),
    );

    const { result } = renderHook(() => useSemaphoreMessages(), {
      wrapper: createWrapper('tester@google.com', 'android:postsubmit'),
    });

    await waitFor(() =>
      expect(result.current).toEqual([
        { name: 'messages/1', longMessage: 'Android outage', urgency: 'FAIL' },
      ]),
    );
  });

  it('hides cached Android messages when switching to non-Android realms or non-Googler auth', async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({
        messages: [
          {
            name: 'messages/1',
            longMessage: 'Cached Android outage',
            urgency: 'FAIL',
          },
        ],
      }),
    );

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    let email = 'tester@google.com';
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>
        <FakeAuthStateProvider value={{ identity: `user:${email}`, email }}>
          <>{children}</>
        </FakeAuthStateProvider>
      </QueryClientProvider>
    );

    const { result, rerender } = renderHook(
      ({ realm }: { realm?: string }) => useSemaphoreMessages(realm),
      {
        initialProps: { realm: 'android:presubmit' as string | undefined },
        wrapper,
      },
    );

    await waitFor(() => expect(result.current).toHaveLength(1));

    rerender({ realm: 'chromium:ci' });
    expect(result.current).toEqual([]);

    rerender({ realm: undefined });
    expect(result.current).toEqual([]);

    email = 'user@example.com';
    rerender({ realm: 'android:presubmit' });
    expect(result.current).toEqual([]);
  });

  it('polls every 4 minutes and disables QueryClient retries on failure', async () => {
    jest.useFakeTimers();
    mockFetch.mockResolvedValue(
      jsonResponse({
        messages: [
          { name: 'messages/1', longMessage: 'First', urgency: 'WARN' },
        ],
      }),
    );

    const { result, unmount } = renderHook(
      () => useSemaphoreMessages('android:presubmit'),
      { wrapper: createWrapper() },
    );

    await waitFor(() => expect(result.current).toHaveLength(1));
    expect(mockFetch).toHaveBeenCalledTimes(1);

    mockFetch.mockResolvedValue(
      jsonResponse({
        messages: [
          { name: 'messages/1', longMessage: 'First', urgency: 'WARN' },
          { name: 'messages/2', longMessage: 'Second', urgency: 'FAIL' },
        ],
      }),
    );

    await act(async () => {
      jest.advanceTimersByTime(SEMAPHORE_POLL_INTERVAL_MS);
    });

    await waitFor(() => expect(result.current).toHaveLength(2));
    unmount();
    jest.useRealTimers();

    mockFetch.mockReset();
    mockFetch.mockResolvedValue({ ok: false, status: 403 } as Response);
    const retryClient = new QueryClient({
      defaultOptions: { queries: { retry: 3, retryDelay: 0 } },
    });

    const { result: failedResult } = renderHook(
      () => useSemaphoreMessages('android:presubmit'),
      {
        wrapper: ({ children }: { children: ReactNode }) => (
          <QueryClientProvider client={retryClient}>
            <FakeAuthStateProvider
              value={{
                identity: 'user:tester@google.com',
                email: 'tester@google.com',
              }}
            >
              <>{children}</>
            </FakeAuthStateProvider>
          </QueryClientProvider>
        ),
      },
    );

    await waitFor(() => expect(mockFetch).toHaveBeenCalledTimes(1));
    expect(failedResult.current).toEqual([]);
  });
});
