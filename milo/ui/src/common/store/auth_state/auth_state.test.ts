// Copyright 2022 The LUCI Authors.
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

import { afterEach, beforeEach, expect, jest } from '@jest/globals';
import { applySnapshot, destroy } from 'mobx-state-tree';

import {
  AuthState,
  queryAuthState,
  setAuthStateCache,
} from '@/common/api/auth_state';

import { AuthStateStore, AuthStateStoreInstance } from './auth_state';

describe('AuthStateStore', () => {
  let store: AuthStateStoreInstance;
  let setAuthStateCacheStub: jest.Mock<
    (authState: AuthState | null) => Promise<void>
  >;
  let queryAuthStateStub: jest.Mock<() => Promise<AuthState>>;

  beforeEach(() => {
    jest.useFakeTimers();
    store = AuthStateStore.create({});
    setAuthStateCacheStub = jest.fn(setAuthStateCache);
    queryAuthStateStub = jest.fn(queryAuthState);
    store.setDependencies({
      setAuthStateCache: setAuthStateCacheStub,
      queryAuthState: queryAuthStateStub,
    });
  });

  afterEach(() => {
    destroy(store);
    jest.useRealTimers();
  });

  describe('scheduleUpdate', () => {
    test("when forceUpdate is false and there's no existing value", async () => {
      const authState = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'token',
        // 1 hour from now.
        accessTokenExpiry: jest.now() / 1000 + 3600,
      };
      queryAuthStateStub.mockResolvedValueOnce(authState);

      const startTime = jest.now();
      store.scheduleUpdate(false);

      await jest.runAllTimersAsync();
      expect(jest.now()).toStrictEqual(startTime);
      expect(queryAuthStateStub.mock.calls.length).toStrictEqual(1);
      expect(setAuthStateCacheStub.mock.calls.length).toStrictEqual(1);
      expect(setAuthStateCacheStub.mock.calls[0]).toEqual([authState]);
    });

    test("when forceUpdate is false and there's an existing value", async () => {
      const authState = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'token',
        // 30 mins from now.
        accessTokenExpiry: jest.now() / 1000 + 1800,
      };
      applySnapshot(store, { id: store.id, value: authState });

      const startTime = jest.now();
      store.scheduleUpdate(false);

      // Advance the timer by 15 min.
      await jest.advanceTimersByTimeAsync(600000);
      expect(queryAuthStateStub.mock.calls.length).toStrictEqual(0);
      expect(setAuthStateCacheStub.mock.calls.length).toStrictEqual(0);

      const refreshedAuthState = {
        ...authState,
        accessToken: 'token2',
        // 1 hour from now.
        accessTokenExpiry: jest.now() / 1000 + 3600,
      };
      queryAuthStateStub.mockResolvedValueOnce(refreshedAuthState);

      await jest.runAllTimersAsync();
      expect(jest.now()).toBeLessThan(startTime + 1800000);
      expect(queryAuthStateStub.mock.calls.length).toStrictEqual(1);
      expect(setAuthStateCacheStub.mock.calls.length).toStrictEqual(1);
      expect(setAuthStateCacheStub.mock.calls[0]).toEqual([refreshedAuthState]);
    });

    test("when forceUpdate is true and there's no existing value", async () => {
      const authState = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'token',
        // 1 hour from now.
        accessTokenExpiry: jest.now() / 1000 + 3600,
      };
      queryAuthStateStub.mockResolvedValueOnce(authState);

      const startTime = jest.now();
      store.scheduleUpdate(true);

      await jest.runAllTimersAsync();
      expect(jest.now()).toStrictEqual(startTime);
      expect(queryAuthStateStub.mock.calls.length).toStrictEqual(1);
      expect(setAuthStateCacheStub.mock.calls.length).toStrictEqual(1);
      expect(setAuthStateCacheStub.mock.calls[0]).toEqual([authState]);
    });

    test("when forceUpdate is true and there's an existing value", async () => {
      const authState = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'token',
        // 30 mins from now.
        accessTokenExpiry: jest.now() / 1000 + 1800,
      };
      applySnapshot(store, { id: store.id, value: authState });

      const refreshedAuthState = {
        ...authState,
        accessToken: 'token2',
        // 1 hour from now.
        accessTokenExpiry: jest.now() / 1000 + 3600,
      };
      queryAuthStateStub.mockResolvedValueOnce(refreshedAuthState);

      const startTime = jest.now();
      store.scheduleUpdate(true);

      await jest.runAllTimersAsync();
      expect(jest.now()).toStrictEqual(startTime);
      expect(queryAuthStateStub.mock.calls.length).toStrictEqual(1);
      expect(setAuthStateCacheStub.mock.calls.length).toStrictEqual(1);
      expect(setAuthStateCacheStub.mock.calls[0]).toEqual([refreshedAuthState]);
    });
  });

  describe('init', () => {
    test("should update token immediately even when it's still valid", async () => {
      const startTime = jest.now();
      store.init({
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'cached-token',
        // 30 mins from now.
        accessTokenExpiry: jest.now() / 1000 + 30 * 60,
      });

      const refreshedAuthState = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'token',
        // 60 mins from now.
        accessTokenExpiry: jest.now() / 1000 + 60 * 60,
      };
      queryAuthStateStub.mockResolvedValueOnce(refreshedAuthState);

      await jest.runOnlyPendingTimersAsync();
      expect(jest.now()).toStrictEqual(startTime);
      expect(queryAuthStateStub.mock.calls.length).toStrictEqual(1);
      expect(setAuthStateCacheStub.mock.calls.length).toStrictEqual(1);
      expect(store.value).toMatchObject(refreshedAuthState);
    });

    test('when getAuthState always return short-lived token', async () => {
      const startTime = jest.now();
      store.init({
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'cached-token',
        // 1 second from now.
        accessTokenExpiry: jest.now() / 1000 + 1,
      });

      let refreshedAuthState = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'token',
        // 2 second from now.
        accessTokenExpiry: jest.now() / 1000 + 2,
      };
      queryAuthStateStub.mockResolvedValueOnce(refreshedAuthState);

      await jest.runOnlyPendingTimersAsync();
      expect(jest.now()).toStrictEqual(startTime);
      expect(queryAuthStateStub.mock.calls.length).toStrictEqual(1);
      expect(setAuthStateCacheStub.mock.calls.length).toStrictEqual(1);
      expect(store.value).toMatchObject(refreshedAuthState);

      // Nothing should refresh within the next 7s.
      await jest.advanceTimersByTimeAsync(7000);
      expect(queryAuthStateStub.mock.calls.length).toStrictEqual(1);
      expect(setAuthStateCacheStub.mock.calls.length).toStrictEqual(1);
      expect(store.value).toMatchObject(refreshedAuthState);

      refreshedAuthState = {
        ...refreshedAuthState,
        accessToken: 'token2',
        // 1 second from now.
        accessTokenExpiry: jest.now() / 1000 + 1,
      };
      queryAuthStateStub.mockResolvedValueOnce(refreshedAuthState);

      // Should refresh within the next 5s
      await jest.advanceTimersByTimeAsync(5000);
      expect(queryAuthStateStub.mock.calls.length).toStrictEqual(2);
      expect(setAuthStateCacheStub.mock.calls.length).toStrictEqual(2);
      expect(store.value).toMatchObject(refreshedAuthState);

      // Nothing should refresh within the next 5s.
      await jest.advanceTimersByTimeAsync(5000);
      expect(queryAuthStateStub.mock.calls.length).toStrictEqual(2);
      expect(setAuthStateCacheStub.mock.calls.length).toStrictEqual(2);
      expect(store.value).toMatchObject(refreshedAuthState);

      refreshedAuthState = {
        ...refreshedAuthState,
        accessToken: 'token3',
        // 1 second from now.
        accessTokenExpiry: jest.now() / 1000 + 1,
      };
      queryAuthStateStub.mockResolvedValueOnce(refreshedAuthState);

      // Should refresh within 5s.
      await jest.advanceTimersByTimeAsync(5000);
      expect(queryAuthStateStub.mock.calls.length).toStrictEqual(3);
      expect(setAuthStateCacheStub.mock.calls.length).toStrictEqual(3);
      expect(store.value).toMatchObject(refreshedAuthState);
    });
  });
});
