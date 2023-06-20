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

import { expect, jest } from '@jest/globals';
import { act, cleanup, render } from '@testing-library/react';
import { applySnapshot, destroy } from 'mobx-state-tree';

import * as authStateLib from '@/common/api/auth_state';
import { Store, StoreInstance, StoreProvider } from '@/common/store';

import {
  AuthStateProvider,
  useAuthState,
  useGetAccessToken,
  useGetIdToken,
} from './auth_state_provider';

interface TokenConsumerProps {
  readonly renderCallback: (
    getIdToken: ReturnType<typeof useGetIdToken>,
    getAccessToken: ReturnType<typeof useGetAccessToken>
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
  const actual = jest.requireActual(
    '@/common/api/auth_state'
  ) as typeof authStateLib;
  const mocked: typeof authStateLib = {
    ...actual,
    // Wraps `queryAuthState` in a mock so we can mock its implementation later.
    queryAuthState: jest.fn(actual.queryAuthState),
  };
  return mocked;
});

const AUTH_STATE = {
  identity: 'identity-1',
  idToken: 'id-token-1',
  accessToken: 'access-token-1',
};

describe('AuthStateProvider', () => {
  let store: StoreInstance;
  let queryAuthStateSpy: jest.Mock<typeof authStateLib.queryAuthState>;

  beforeEach(() => {
    store = Store.create({});
    jest.useFakeTimers();
    queryAuthStateSpy = authStateLib.queryAuthState as jest.Mock<
      typeof authStateLib.queryAuthState
    >;
    queryAuthStateSpy.mockResolvedValue(AUTH_STATE);
  });
  afterEach(() => {
    queryAuthStateSpy.mockRestore();
    cleanup();
    destroy(store);
    jest.useRealTimers();
  });

  it('e2e', async () => {
    const tokenConsumerCBSpy = jest.fn(
      (
        _getIdToken: ReturnType<typeof useGetIdToken>,
        _getAccessToken: ReturnType<typeof useGetAccessToken>
      ) => {}
    );
    const identityConsumerCBSpy = jest.fn((_identity: string) => {});

    render(
      <StoreProvider value={store}>
        <AuthStateProvider initialValue={AUTH_STATE}>
          <IdentityConsumer renderCallback={identityConsumerCBSpy} />
          <TokenConsumer renderCallback={tokenConsumerCBSpy} />
        </AuthStateProvider>
      </StoreProvider>
    );

    await jest.runAllTimersAsync();

    expect(identityConsumerCBSpy.mock.calls.length).toStrictEqual(1);
    expect(identityConsumerCBSpy.mock.lastCall?.[0]).toStrictEqual(
      'identity-1'
    );
    expect(tokenConsumerCBSpy.mock.calls.length).toStrictEqual(1);
    expect(tokenConsumerCBSpy.mock.lastCall?.[0]()).toStrictEqual('id-token-1');
    expect(tokenConsumerCBSpy.mock.lastCall?.[1]()).toStrictEqual(
      'access-token-1'
    );

    // Update tokens but not identity.
    act(() => {
      applySnapshot(store.authState, {
        id: store.authState.id,
        value: {
          identity: 'identity-1',
          idToken: 'id-token-2',
          accessToken: 'access-token-2',
        },
      });
    });
    await jest.runAllTimersAsync();

    // Update tokens should not trigger context updates.
    expect(identityConsumerCBSpy.mock.calls.length).toStrictEqual(1);
    expect(identityConsumerCBSpy.mock.lastCall?.[0]).toStrictEqual(
      'identity-1'
    );
    expect(tokenConsumerCBSpy.mock.calls.length).toStrictEqual(1);
    // The token getters can still return the latest tokens.
    expect(tokenConsumerCBSpy.mock.lastCall?.[0]()).toStrictEqual('id-token-2');
    expect(tokenConsumerCBSpy.mock.lastCall?.[1]()).toStrictEqual(
      'access-token-2'
    );

    // Update identity and tokens.
    act(() => {
      applySnapshot(store.authState, {
        id: store.authState.id,
        value: {
          identity: 'identity-2',
          idToken: 'id-token-3',
          accessToken: 'access-token-3',
        },
      });
    });
    await jest.runAllTimersAsync();

    // Update identity should trigger context updates.
    expect(identityConsumerCBSpy.mock.calls.length).toStrictEqual(2);
    expect(identityConsumerCBSpy.mock.lastCall?.[0]).toStrictEqual(
      'identity-2'
    );
    expect(tokenConsumerCBSpy.mock.calls.length).toStrictEqual(2);
    expect(tokenConsumerCBSpy.mock.lastCall?.[0]()).toStrictEqual('id-token-3');
    expect(tokenConsumerCBSpy.mock.lastCall?.[1]()).toStrictEqual(
      'access-token-3'
    );
  });
});
