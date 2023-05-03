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
import { applySnapshot, destroy } from 'mobx-state-tree';
import * as sinon from 'sinon';

import { getAuthStateCache, queryAuthState, setAuthStateCache } from '../libs/auth_state';
import { StubFn, stubFn } from '../libs/test_utils/sinon';
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
    it("when forceUpdate is false and there's no existing value", async () => {
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

    it("when forceUpdate is false and there's an existing value", async () => {
      const authState = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'token',
        // 30 mins from now.
        accessTokenExpiry: timer.now / 1000 + 1800,
      };
      applySnapshot(store, { id: store.id, value: authState });

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

    it("when forceUpdate is true and there's no existing value", async () => {
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

    it("when forceUpdate is true and there's an existing value", async () => {
      const authState = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'token',
        // 30 mins from now.
        accessTokenExpiry: timer.now / 1000 + 1800,
      };
      applySnapshot(store, { id: store.id, value: authState });

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
    it('when getAuthState always return short-lived token', async () => {
      getAuthStateCacheStub.onFirstCall().resolves(null);

      const startTime = timer.now;
      store.init({
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'cached-token',
        // 1 second from now.
        accessTokenExpiry: timer.now / 1000 + 1,
      });

      let refreshedAuthState = {
        identity: 'user:user@google.com',
        email: 'user@google.com',
        accessToken: 'token',
        // 2 second from now.
        accessTokenExpiry: timer.now / 1000 + 2,
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
