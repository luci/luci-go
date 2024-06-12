// Copyright 2023 The LUCI Authors.
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
import { cleanup, render } from '@testing-library/react';
import { DateTime } from 'luxon';
import { destroy } from 'mobx-state-tree';
import { act } from 'react';

import * as authStateLib from '@/common/api/auth_state';
import { Store, StoreInstance, StoreProvider } from '@/common/store';
import { timeout } from '@/generic_libs/tools/utils';

import { AuthStateProvider } from './auth_state_provider';
import { useAuthState, useGetAccessToken, useGetIdToken } from './context';

interface TokenConsumerProps {
  readonly renderCallback: (
    getIdToken: ReturnType<typeof useGetIdToken>,
    getAccessToken: ReturnType<typeof useGetAccessToken>,
  ) => void;
}

function TokenConsumer({ renderCallback }: TokenConsumerProps) {
  const getIdToken = useGetIdToken();
  const getAccessToken = useGetAccessToken();
  renderCallback(getIdToken, getAccessToken);

  return <></>;
}

interface IdentityConsumerProps {
  readonly renderCallback: (identity: string) => void;
}

function IdentityConsumer({ renderCallback }: IdentityConsumerProps) {
  const authState = useAuthState();
  renderCallback(authState.identity);

  return <></>;
}

jest.mock('@/common/api/auth_state', () => {
  return createSelectiveMockFromModule<
    typeof import('@/common/api/auth_state')
  >('@/common/api/auth_state', ['queryAuthState']);
});

describe('<AuthStateProvider />', () => {
  let store: StoreInstance;
  let queryAuthStateSpy: jest.MockedFunction<
    typeof authStateLib.queryAuthState
  >;

  beforeEach(() => {
    store = Store.create({});
    jest.useFakeTimers();
    queryAuthStateSpy = jest.mocked(authStateLib.queryAuthState);
  });
  afterEach(() => {
    cleanup();
    queryAuthStateSpy.mockReset();
    destroy(store);
    jest.useRealTimers();
  });

  it('should refresh auth state correctly', async () => {
    const tokenConsumerCBSpy = jest.fn(
      (
        _getIdToken: ReturnType<typeof useGetIdToken>,
        _getAccessToken: ReturnType<typeof useGetAccessToken>,
      ) => {},
    );
    const identityConsumerCBSpy = jest.fn((_identity: string) => {});
    const initialAuthState = {
      identity: 'identity-1',
      idToken: 'id-token-1',
      accessToken: 'access-token-1',
      accessTokenExpiry: DateTime.now().plus({ minute: 20 }).toSeconds(),
    };
    const firstQueryResponse = {
      identity: 'identity-1',
      idToken: 'id-token-2',
      accessToken: 'access-token-2',
      accessTokenExpiry: DateTime.now().plus({ minute: 60 }).toSeconds(),
    };
    queryAuthStateSpy.mockResolvedValue(
      // Resolve after 1s.
      timeout(1000).then(() => firstQueryResponse),
    );

    render(
      <QueryClientProvider client={new QueryClient()}>
        <StoreProvider value={store}>
          <AuthStateProvider initialValue={initialAuthState}>
            <IdentityConsumer renderCallback={identityConsumerCBSpy} />
            <TokenConsumer renderCallback={tokenConsumerCBSpy} />
          </AuthStateProvider>
        </StoreProvider>
      </QueryClientProvider>,
    );

    // Advance timer by 500ms, before the first query returns.
    await act(() => jest.advanceTimersByTimeAsync(500));
    // The first query should've been sent immediately. Even when the initial
    // auth token hasn't expired yet. This is necessary because the initial
    // value could've been an outdated cache (if user signed in/out in a
    // different tab)
    expect(queryAuthStateSpy).toHaveBeenCalledTimes(1);
    expect(identityConsumerCBSpy.mock.calls.length).toStrictEqual(1);
    expect(identityConsumerCBSpy.mock.lastCall?.[0]).toStrictEqual(
      'identity-1',
    );
    expect(tokenConsumerCBSpy.mock.calls.length).toStrictEqual(1);
    expect(await tokenConsumerCBSpy.mock.lastCall?.[0]()).toStrictEqual(
      'id-token-1',
    );
    expect(await tokenConsumerCBSpy.mock.lastCall?.[1]()).toStrictEqual(
      'access-token-1',
    );
    expect(store.authState.value).toEqual(initialAuthState);
    expect(authStateLib.getAuthStateCacheSync()).toBeNull();

    // Advance timer by 40s, after the initial query returns but before the
    // initial token expires.
    await act(() => jest.advanceTimersByTimeAsync(40000));
    // Update tokens should not trigger context updates.
    expect(identityConsumerCBSpy.mock.calls.length).toStrictEqual(1);
    expect(identityConsumerCBSpy.mock.lastCall?.[0]).toStrictEqual(
      'identity-1',
    );
    expect(tokenConsumerCBSpy.mock.calls.length).toStrictEqual(1);
    // The token getters can still return the latest tokens.
    expect(await tokenConsumerCBSpy.mock.lastCall?.[0]()).toStrictEqual(
      'id-token-2',
    );
    expect(await tokenConsumerCBSpy.mock.lastCall?.[1]()).toStrictEqual(
      'access-token-2',
    );
    expect(store.authState.value).toEqual(firstQueryResponse);
    expect(authStateLib.getAuthStateCacheSync()).toEqual(firstQueryResponse);

    const secondQueryResponse = {
      identity: 'identity-2',
      idToken: 'id-token-3',
      accessToken: 'access-token-3',
      accessTokenExpiry: DateTime.fromSeconds(
        firstQueryResponse.accessTokenExpiry,
      )
        .plus({ minute: 29 })
        .toSeconds(),
    };
    queryAuthStateSpy.mockResolvedValue(secondQueryResponse);

    // Advance the timer to just before the first queried token is about to
    // expire.
    await act(() =>
      jest.advanceTimersByTimeAsync(
        firstQueryResponse.accessTokenExpiry * 1000 - Date.now() - 10000,
      ),
    );

    expect(queryAuthStateSpy).toHaveBeenCalledTimes(2);
    // Update identity should trigger context updates.
    expect(identityConsumerCBSpy.mock.calls.length).toStrictEqual(2);
    expect(identityConsumerCBSpy.mock.lastCall?.[0]).toStrictEqual(
      'identity-2',
    );
    expect(tokenConsumerCBSpy.mock.calls.length).toStrictEqual(2);
    // The token getters can still return the latest tokens.
    expect(await tokenConsumerCBSpy.mock.lastCall?.[0]()).toStrictEqual(
      'id-token-3',
    );
    expect(await tokenConsumerCBSpy.mock.lastCall?.[1]()).toStrictEqual(
      'access-token-3',
    );
    expect(store.authState.value).toEqual(secondQueryResponse);
    expect(authStateLib.getAuthStateCacheSync()).toEqual(secondQueryResponse);

    const thirdQueryResponse = {
      identity: 'identity-2',
      idToken: 'id-token-4',
      accessToken: 'access-token-4',
      accessTokenExpiry: DateTime.fromSeconds(
        secondQueryResponse.accessTokenExpiry,
      )
        .plus({ minute: 59 })
        .toSeconds(),
    };
    queryAuthStateSpy.mockResolvedValue(thirdQueryResponse);

    // Advance the timer to just before the second queried token is about to
    // expire.
    await act(() =>
      jest.advanceTimersByTimeAsync(
        secondQueryResponse.accessTokenExpiry * 1000 - Date.now() - 10000,
      ),
    );

    expect(queryAuthStateSpy).toHaveBeenCalledTimes(3);
    // Update identity should trigger context updates.
    expect(identityConsumerCBSpy.mock.calls.length).toStrictEqual(2);
    expect(identityConsumerCBSpy.mock.lastCall?.[0]).toStrictEqual(
      'identity-2',
    );
    expect(tokenConsumerCBSpy.mock.calls.length).toStrictEqual(2);
    // The token getters can still return the latest tokens.
    expect(await tokenConsumerCBSpy.mock.lastCall?.[0]()).toStrictEqual(
      'id-token-4',
    );
    expect(await tokenConsumerCBSpy.mock.lastCall?.[1]()).toStrictEqual(
      'access-token-4',
    );
    expect(store.authState.value).toEqual(thirdQueryResponse);
    expect(authStateLib.getAuthStateCacheSync()).toEqual(thirdQueryResponse);
  });

  it('should not return expired tokens', async () => {
    const tokenConsumerCBSpy = jest.fn(
      (
        _getIdToken: ReturnType<typeof useGetIdToken>,
        _getAccessToken: ReturnType<typeof useGetAccessToken>,
      ) => {},
    );
    const initialAuthState = {
      identity: 'identity-1',
      idToken: 'id-token-1',
      accessToken: 'access-token-1',
      accessTokenExpiry: DateTime.now().plus({ minute: -20 }).toSeconds(),
    };
    queryAuthStateSpy.mockResolvedValueOnce({
      identity: 'identity-1',
      idToken: 'id-token-2',
      accessToken: 'access-token-2',
      accessTokenExpiry: DateTime.now().plus({ minute: -10 }).toSeconds(),
    });
    queryAuthStateSpy.mockResolvedValueOnce({
      identity: 'identity-1',
      idToken: 'id-token-3',
      accessToken: 'access-token-3',
      accessTokenExpiry: DateTime.now().plus({ minute: 10 }).toSeconds(),
    });

    render(
      <QueryClientProvider client={new QueryClient()}>
        <StoreProvider value={store}>
          <AuthStateProvider initialValue={initialAuthState}>
            <TokenConsumer renderCallback={tokenConsumerCBSpy} />
          </AuthStateProvider>
        </StoreProvider>
      </QueryClientProvider>,
    );

    expect(tokenConsumerCBSpy.mock.calls.length).toStrictEqual(1);
    const getIdTokenPromise = tokenConsumerCBSpy.mock.lastCall![0]();
    const getAccessTokenPromise = tokenConsumerCBSpy.mock.lastCall![1]();

    // Ensure that the non-expired tokens haven't been returned when calling the
    // token getters.
    expect(queryAuthStateSpy.mock.calls.length).toBeLessThan(2);

    // Advance timer by 1min.
    await act(() => jest.advanceTimersByTimeAsync(60000));
    expect(await getIdTokenPromise).toStrictEqual('id-token-3');
    expect(await getAccessTokenPromise).toStrictEqual('access-token-3');
  });

  it('should not return tokens for another identity', async () => {
    const tokenConsumerCBSpy = jest.fn(
      (
        _getIdToken: ReturnType<typeof useGetIdToken>,
        _getAccessToken: ReturnType<typeof useGetAccessToken>,
      ) => {},
    );
    const initialAuthState = {
      identity: 'identity-1',
      idToken: 'id-token-1',
      accessToken: 'access-token-1',
      accessTokenExpiry: DateTime.now().plus({ minute: -20 }).toSeconds(),
    };
    queryAuthStateSpy.mockResolvedValueOnce({
      identity: 'identity-1',
      idToken: 'id-token-2',
      accessToken: 'access-token-2',
      accessTokenExpiry: DateTime.now().plus({ minute: -10 }).toSeconds(),
    });
    queryAuthStateSpy.mockResolvedValueOnce({
      identity: 'identity-2',
      idToken: 'id-token-3',
      accessToken: 'access-token-3',
      accessTokenExpiry: DateTime.now().plus({ minute: 10 }).toSeconds(),
    });

    render(
      <QueryClientProvider client={new QueryClient()}>
        <StoreProvider value={store}>
          <AuthStateProvider initialValue={initialAuthState}>
            <TokenConsumer renderCallback={tokenConsumerCBSpy} />
          </AuthStateProvider>
        </StoreProvider>
      </QueryClientProvider>,
    );

    expect(tokenConsumerCBSpy.mock.calls.length).toStrictEqual(1);
    let resolvedIdToken: string | null = null;
    let resolvedAccessToken: string | null = null;
    tokenConsumerCBSpy.mock
      .lastCall![0]()
      .then((tok) => (resolvedIdToken = tok));
    tokenConsumerCBSpy.mock
      .lastCall![1]()
      .then((tok) => (resolvedAccessToken = tok));

    // Ensure that the non-expired tokens haven't been returned when calling the
    // token getters.
    expect(queryAuthStateSpy.mock.calls.length).toBeLessThan(2);

    queryAuthStateSpy.mockResolvedValue({
      identity: 'identity-2',
      idToken: 'id-token-3',
      accessToken: 'access-token-3',
      accessTokenExpiry: DateTime.now().plus({ minute: 10 }).toSeconds(),
    });

    // Advance timer by several hours.
    await act(() => jest.advanceTimersByTimeAsync(3600000));
    await act(() => jest.advanceTimersByTimeAsync(3600000));
    await act(() => jest.advanceTimersByTimeAsync(3600000));

    expect(resolvedIdToken).toBeNull();
    expect(resolvedAccessToken).toBeNull();
  });

  it('should not update auth state too frequently when tokens are very short lived', async () => {
    queryAuthStateSpy.mockImplementation(async () => {
      return {
        identity: 'identity',
        idToken: 'id-token',
        accessToken: 'access-token',
        idTokenExpiry: DateTime.now().plus({ seconds: 1 }).toSeconds(),
      };
    });

    render(
      <QueryClientProvider client={new QueryClient()}>
        <StoreProvider value={store}>
          <AuthStateProvider
            initialValue={{
              identity: 'identity-1',
              idToken: 'id-token-1',
              accessToken: 'access-token-1',
              idTokenExpiry: DateTime.now().plus({ seconds: 1 }).toSeconds(),
            }}
          >
            <></>
          </AuthStateProvider>
        </StoreProvider>
      </QueryClientProvider>,
    );

    // In the first 3 seconds, there should only be one query.
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(queryAuthStateSpy).toHaveBeenCalledTimes(1);
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(queryAuthStateSpy).toHaveBeenCalledTimes(1);
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(queryAuthStateSpy).toHaveBeenCalledTimes(1);

    // Move the timer to when the next query is sent.
    await act(() => jest.advanceTimersToNextTimerAsync());
    expect(queryAuthStateSpy).toHaveBeenCalledTimes(2);

    // In the next 3 seconds, no more query should be sent.
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(queryAuthStateSpy).toHaveBeenCalledTimes(2);
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(queryAuthStateSpy).toHaveBeenCalledTimes(2);
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(queryAuthStateSpy).toHaveBeenCalledTimes(2);
  });

  it("should not refetch auth-state when tokens don't expire", async () => {
    queryAuthStateSpy.mockImplementation(async () => {
      return {
        identity: 'identity',
        idToken: 'id-token',
        accessToken: 'access-token',
      };
    });

    render(
      <QueryClientProvider client={new QueryClient()}>
        <StoreProvider value={store}>
          <AuthStateProvider
            initialValue={{
              identity: 'identity-1',
              idToken: 'id-token-1',
              accessToken: 'access-token-1',
            }}
          >
            <></>
          </AuthStateProvider>
        </StoreProvider>
      </QueryClientProvider>,
    );

    // In the first 3 seconds, there should be only one query.
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(queryAuthStateSpy).toHaveBeenCalledTimes(1);

    // Move the timer by several hours, no more query should be sent.
    await act(() => jest.advanceTimersByTimeAsync(3600000));
    await act(() => jest.advanceTimersByTimeAsync(3600000));
    await act(() => jest.advanceTimersByTimeAsync(3600000));
    expect(queryAuthStateSpy).toHaveBeenCalledTimes(1);
  });

  it('token getters should be referentially stable', async () => {
    // Set up mocks.
    let callCount = 0;
    queryAuthStateSpy.mockImplementation(async () => {
      callCount += 1;
      return {
        identity: 'identity',
        idToken: `id-token-${callCount}`,
        accessToken: `access-token-${callCount}`,
        accessTokenExpiry: DateTime.now().plus({ minute: 60 }).toSeconds(),
      };
    });
    const tokenConsumerCBSpy = jest.fn(
      (
        _getIdToken: ReturnType<typeof useGetIdToken>,
        _getAccessToken: ReturnType<typeof useGetAccessToken>,
      ) => {},
    );
    const initialAuthState = {
      identity: 'identity',
      idToken: 'id-token-0',
      accessToken: 'access-token-0',
      accessTokenExpiry: DateTime.now().plus({ minute: 60 }).toSeconds(),
    };

    // First render.
    const { rerender } = render(
      <QueryClientProvider client={new QueryClient()}>
        <StoreProvider value={store}>
          <AuthStateProvider initialValue={initialAuthState}>
            <TokenConsumer renderCallback={tokenConsumerCBSpy} />
          </AuthStateProvider>
        </StoreProvider>
      </QueryClientProvider>,
    );

    // Wait until the first token refresh is complete.
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(queryAuthStateSpy).toHaveBeenCalledTimes(1);
    expect(tokenConsumerCBSpy).toHaveBeenCalledTimes(1);

    // Trigger a rerender.
    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <StoreProvider value={store}>
          <AuthStateProvider initialValue={initialAuthState}>
            <TokenConsumer renderCallback={tokenConsumerCBSpy} />
          </AuthStateProvider>
        </StoreProvider>
      </QueryClientProvider>,
    );

    // Verify that the token getters are referentially stable.
    expect(tokenConsumerCBSpy).toHaveBeenCalledTimes(2);
    expect(tokenConsumerCBSpy.mock.calls[0][0]).toStrictEqual(
      tokenConsumerCBSpy.mock.calls[1][0],
    );
    expect(tokenConsumerCBSpy.mock.calls[0][1]).toStrictEqual(
      tokenConsumerCBSpy.mock.calls[1][1],
    );

    // Wait until the 2nd token refresh is complete.
    await act(() => jest.advanceTimersByTimeAsync(3600000));
    expect(queryAuthStateSpy).toHaveBeenCalledTimes(2);
    expect(tokenConsumerCBSpy).toHaveBeenCalledTimes(2);

    // Trigger another rerender.
    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <StoreProvider value={store}>
          <AuthStateProvider initialValue={initialAuthState}>
            <TokenConsumer renderCallback={tokenConsumerCBSpy} />
          </AuthStateProvider>
        </StoreProvider>
      </QueryClientProvider>,
    );

    // Verify that the token getters are still referentially stable.
    expect(tokenConsumerCBSpy).toHaveBeenCalledTimes(3);
    expect(tokenConsumerCBSpy.mock.calls[1][0]).toStrictEqual(
      tokenConsumerCBSpy.mock.calls[2][0],
    );
    expect(tokenConsumerCBSpy.mock.calls[1][1]).toStrictEqual(
      tokenConsumerCBSpy.mock.calls[2][1],
    );

    // Change identity.
    queryAuthStateSpy.mockImplementation(async () => {
      callCount += 1;
      return {
        identity: 'identity-2',
        idToken: `id-token-${callCount}`,
        accessToken: `access-token-${callCount}`,
        accessTokenExpiry: DateTime.now().plus({ minute: 60 }).toSeconds(),
      };
    });

    // Wait until the 3rd token refresh is complete.
    await act(() => jest.advanceTimersByTimeAsync(3600000));
    expect(queryAuthStateSpy).toHaveBeenCalledTimes(3);

    // Verify that the token getters are changed after the identity is changed.
    expect(tokenConsumerCBSpy).toHaveBeenCalledTimes(4);
    expect(tokenConsumerCBSpy.mock.calls[2][0]).not.toStrictEqual(
      tokenConsumerCBSpy.mock.calls[3][0],
    );
    expect(tokenConsumerCBSpy.mock.calls[2][1]).not.toStrictEqual(
      tokenConsumerCBSpy.mock.calls[3][1],
    );

    // Trigger a rerender after the identity change.
    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <StoreProvider value={store}>
          <AuthStateProvider initialValue={initialAuthState}>
            <TokenConsumer renderCallback={tokenConsumerCBSpy} />
          </AuthStateProvider>
        </StoreProvider>
      </QueryClientProvider>,
    );

    // Verify that the token getters are referentially stable for the new
    // identity.
    expect(tokenConsumerCBSpy).toHaveBeenCalledTimes(5);
    expect(tokenConsumerCBSpy.mock.calls[3][0]).toStrictEqual(
      tokenConsumerCBSpy.mock.calls[4][0],
    );
    expect(tokenConsumerCBSpy.mock.calls[3][1]).toStrictEqual(
      tokenConsumerCBSpy.mock.calls[4][1],
    );
  });
});
