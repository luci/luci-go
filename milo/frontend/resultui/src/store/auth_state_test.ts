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

import * as chai from 'chai';
import { expect } from 'chai';
import chaiSubset from 'chai-subset';
import { destroy } from 'mobx-state-tree';
import * as sinon from 'sinon';

import { getAuthStateCache, setAuthStateCache } from '../auth_state_cache';
import { StubFn, stubFn } from '../libs/test_utils/sinon';
import { timeout } from '../libs/utils';
import { queryAuthState } from '../services/milo_internal';
import { AuthStateStore, AuthStateStoreInstance } from './auth_state';

chai.use(chaiSubset);

describe('AuthStateStore', () => {
  let store: AuthStateStoreInstance;
  let timer: sinon.SinonFakeTimers;
  let getAuthStateCacheStub: StubFn<typeof getAuthStateCache>;
  let setAuthStateCacheStub: StubFn<typeof setAuthStateCache>;
  let queryAuthStateStub: StubFn<typeof queryAuthState>;

  beforeEach(() => {
    timer = sinon.useFakeTimers();
    store = AuthStateStore.create({});
    getAuthStateCacheStub = stubFn<typeof getAuthStateCache>();
    setAuthStateCacheStub = stubFn<typeof setAuthStateCache>();
    queryAuthStateStub = stubFn<typeof queryAuthState>();
    store.setDependencies({
      getAuthStateCache: getAuthStateCacheStub,
      setAuthStateCache: setAuthStateCacheStub,
      queryAuthState: queryAuthStateStub,
    });
  });

  afterEach(() => {
    destroy(store);
    timer.restore();
  });

  describe('scheduleUpdate', () => {
    it("when foceUpdate is false and there's no existing value", async () => {
      const authState = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'token',
        // 1 hour from now.
        accessTokenExpiry: timer.now / 1000 + 3600,
      };
      queryAuthStateStub.onFirstCall().resolves(authState);

      const startTime = timer.now;
      store.scheduleUpdate(false);

      expect(await timer.runAllAsync()).to.eq(startTime);
      expect(queryAuthStateStub.callCount).to.eq(1);
      expect(setAuthStateCacheStub.callCount).to.eq(1);
      expect(setAuthStateCacheStub.getCall(0).args).to.deep.eq([authState]);
    });

    it("when foceUpdate is false and there's an existing value", async () => {
      const authState = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'token',
        // 30 mins from now.
        accessTokenExpiry: timer.now / 1000 + 1800,
      };
      store.setValue(authState);

      const startTime = timer.now;
      store.scheduleUpdate(false);

      // Advance the timer by 15 min.
      await timer.tickAsync(600000);
      expect(queryAuthStateStub.callCount).to.eq(0);
      expect(setAuthStateCacheStub.callCount).to.eq(0);

      const refreshedAuthState = {
        ...authState,
        accessToken: 'token2',
        // 1 hour from now.
        accessTokenExpiry: timer.now / 1000 + 3600,
      };
      queryAuthStateStub.onFirstCall().resolves(refreshedAuthState);

      expect(await timer.runAllAsync()).to.lt(startTime + 1800000);
      expect(queryAuthStateStub.callCount).to.eq(1);
      expect(setAuthStateCacheStub.callCount).to.eq(1);
      expect(setAuthStateCacheStub.getCall(0).args).to.deep.eq([refreshedAuthState]);
    });

    it("when foceUpdate is true and there's no existing value", async () => {
      const authState = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'token',
        // 1 hour from now.
        accessTokenExpiry: timer.now / 1000 + 3600,
      };
      queryAuthStateStub.onFirstCall().resolves(authState);

      const startTime = timer.now;
      store.scheduleUpdate(true);

      expect(await timer.runAllAsync()).to.eq(startTime);
      expect(queryAuthStateStub.callCount).to.eq(1);
      expect(setAuthStateCacheStub.callCount).to.eq(1);
      expect(setAuthStateCacheStub.getCall(0).args).to.deep.eq([authState]);
    });

    it("when foceUpdate is true and there's an existing value", async () => {
      const authState = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'token',
        // 30 mins from now.
        accessTokenExpiry: timer.now / 1000 + 1800,
      };
      store.setValue(authState);

      const refreshedAuthState = {
        ...authState,
        accessToken: 'token2',
        // 1 hour from now.
        accessTokenExpiry: timer.now / 1000 + 3600,
      };
      queryAuthStateStub.onFirstCall().resolves(refreshedAuthState);

      const startTime = timer.now;
      store.scheduleUpdate(true);

      expect(await timer.runAllAsync()).to.eq(startTime);
      expect(queryAuthStateStub.callCount).to.eq(1);
      expect(setAuthStateCacheStub.callCount).to.eq(1);
      expect(setAuthStateCacheStub.getCall(0).args).to.deep.eq([refreshedAuthState]);
    });
  });

  describe('init', () => {
    it("when there's no existing value", async () => {
      const authStateCache = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'token',
        // 30 mins from now.
        accessTokenExpiry: timer.now / 1000 + 1800,
      };
      getAuthStateCacheStub.onFirstCall().returns(
        Promise.resolve()
          .then(() => timeout(10))
          .then(() => authStateCache)
      );

      const startTime = timer.now;
      store.init();

      expect(await timer.runToLastAsync()).to.eq(startTime + 10);
      expect(getAuthStateCacheStub.callCount).to.eq(1);
      expect(queryAuthStateStub.callCount).to.eq(0);
      expect(setAuthStateCacheStub.callCount).to.eq(0);
      expect(store.value).to.deep.include(authStateCache);

      const refreshedAuthState = {
        ...authStateCache,
        accessToken: 'token2',
        // 1 hour from now.
        accessTokenExpiry: timer.now / 1000 + 3600,
      };
      queryAuthStateStub.onFirstCall().resolves(refreshedAuthState);

      const refreshedAuthState2 = {
        ...authStateCache,
        accessToken: 'token3',
        // 2 hours from now.
        accessTokenExpiry: timer.now / 1000 + 7200,
      };
      queryAuthStateStub.onSecondCall().resolves(refreshedAuthState2);

      const refreshedAuthState3 = {
        ...authStateCache,
        accessToken: 'token3',
        // 3 hours from now.
        accessTokenExpiry: timer.now / 1000 + 9600,
      };
      queryAuthStateStub.onThirdCall().resolves(refreshedAuthState3);

      await timer.tickAsync(100);
      expect(getAuthStateCacheStub.callCount).to.eq(1);
      expect(queryAuthStateStub.callCount).to.eq(1);
      expect(setAuthStateCacheStub.callCount).to.eq(1);
      expect(setAuthStateCacheStub.getCall(0).args).to.deep.eq([refreshedAuthState]);
      expect(store.value).to.deep.include(refreshedAuthState);

      // Advance clock by 30 mins. The auth state should still be valid.
      // No refresh should've happened.
      await timer.tickAsync(1800000);
      expect(getAuthStateCacheStub.callCount).to.eq(1);
      expect(queryAuthStateStub.callCount).to.eq(1);
      expect(setAuthStateCacheStub.callCount).to.eq(1);

      // Advance clock by another 30 mins. The auth state should've been
      // refreshed.
      await timer.tickAsync(1800000);
      expect(getAuthStateCacheStub.callCount).to.eq(1);
      expect(queryAuthStateStub.callCount).to.eq(2);
      expect(setAuthStateCacheStub.callCount).to.eq(2);
      expect(store.value).to.deep.include(refreshedAuthState2);
      expect(setAuthStateCacheStub.getCall(1).args).to.deep.eq([refreshedAuthState2]);

      // Advance clock by 30 mins. The auth state should still be valid.
      // No refresh should've happened.
      await timer.tickAsync(1800000);
      expect(getAuthStateCacheStub.callCount).to.eq(1);
      expect(queryAuthStateStub.callCount).to.eq(2);
      expect(setAuthStateCacheStub.callCount).to.eq(2);

      // Advance clock by another 30 mins. The auth state should've been
      // refreshed.
      await timer.tickAsync(1800000);
      expect(getAuthStateCacheStub.callCount).to.eq(1);
      expect(queryAuthStateStub.callCount).to.eq(3);
      expect(setAuthStateCacheStub.callCount).to.eq(3);
      expect(store.value).to.deep.include(refreshedAuthState3);
      expect(setAuthStateCacheStub.getCall(2).args).to.deep.eq([refreshedAuthState3]);
    });

    it("when there's an existing value", async () => {
      timer.tick(3600000);

      const authStateCache = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'cached-token',
        // 30 mins from now.
        accessTokenExpiry: timer.now / 1000 + 1800,
      };
      getAuthStateCacheStub.onFirstCall().returns(
        Promise.resolve()
          .then(() => timeout(10))
          .then(() => authStateCache)
      );

      const authState = {
        ...authStateCache,
        accessToken: 'existing-token',
        // 30 mins ago.
        accessTokenExpiry: timer.now / 1000 - 1800,
      };
      store.setValue(authState);

      store.init();

      await timer.runToLastAsync();
      expect(store.value).to.deep.include(authState);
    });

    it('when the value is updated before the cache is applied', async () => {
      const authStateCache = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'cached-token',
        // 30 mins from now.
        accessTokenExpiry: timer.now / 1000 + 1800,
      };
      getAuthStateCacheStub.onFirstCall().returns(
        Promise.resolve()
          .then(() => timeout(10))
          .then(() => authStateCache)
      );

      // Advance the timer a little bit. So the getAuthStateCache resolves has
      // not be resolved.
      await timer.tickAsync(5);

      // Set a value before the cached value is applied.
      const authState = {
        ...authStateCache,
        accessToken: 'existing-token',
        // 1 hour from now.
        accessTokenExpiry: timer.now / 1000 + 3600,
      };
      store.setValue(authState);

      await timer.runToLastAsync();
      expect(store.value).to.deep.include(authState);
    });

    it('when getAuthState always return short-lived token', async () => {
      getAuthStateCacheStub.onFirstCall().resolves(null);

      const startTime = timer.now;
      store.init();

      let refreshedAuthState = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'token',
        // 1 second from now.
        accessTokenExpiry: timer.now / 1000 + 1,
      };
      queryAuthStateStub.onFirstCall().resolves(refreshedAuthState);

      expect(await timer.runToLastAsync()).to.eq(startTime);
      expect(queryAuthStateStub.callCount).to.eq(1);
      expect(setAuthStateCacheStub.callCount).to.eq(1);
      expect(store.value).to.deep.include(refreshedAuthState);

      // Nothing should refresh within the next 7s.
      await timer.tickAsync(7000);
      expect(queryAuthStateStub.callCount).to.eq(1);
      expect(setAuthStateCacheStub.callCount).to.eq(1);
      expect(store.value).to.deep.include(refreshedAuthState);

      refreshedAuthState = {
        ...refreshedAuthState,
        accessToken: 'token2',
        // 1 second from now.
        accessTokenExpiry: timer.now / 1000 + 1,
      };
      queryAuthStateStub.onSecondCall().resolves(refreshedAuthState);

      // Should refresh within the next 5s
      await timer.tickAsync(5000);
      expect(queryAuthStateStub.callCount).to.eq(2);
      expect(setAuthStateCacheStub.callCount).to.eq(2);
      expect(store.value).to.deep.include(refreshedAuthState);

      // Nothing should refresh within the next 5s.
      await timer.tickAsync(5000);
      expect(queryAuthStateStub.callCount).to.eq(2);
      expect(setAuthStateCacheStub.callCount).to.eq(2);
      expect(store.value).to.deep.include(refreshedAuthState);

      refreshedAuthState = {
        ...refreshedAuthState,
        accessToken: 'token3',
        // 1 second from now.
        accessTokenExpiry: timer.now / 1000 + 1,
      };
      queryAuthStateStub.onThirdCall().resolves(refreshedAuthState);

      // Should refresh within 5s.
      await timer.tickAsync(5000);
      expect(queryAuthStateStub.callCount).to.eq(3);
      expect(setAuthStateCacheStub.callCount).to.eq(3);
      expect(store.value).to.deep.include(refreshedAuthState);
    });
  });
});
