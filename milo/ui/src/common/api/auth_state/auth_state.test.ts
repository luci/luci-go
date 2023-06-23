// Copyright 2021 The LUCI Authors.
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

import {
  ANONYMOUS_IDENTITY,
  getAuthStateCache,
  getAuthStateCacheSync,
  msToExpire,
  setAuthStateCache,
} from './auth_state';

describe('auth_state', () => {
  test('support accessing auth state synchronously', () => {
    const state = {
      accessToken: Math.random().toString(),
      identity: `user: ${Math.random()}`,
      accessTokenExpiry: Date.now() / 1000 + 1000,
    };
    setAuthStateCache(state);
    expect(getAuthStateCacheSync()).toEqual(state);
  });

  test('support accessing auth state asynchronously', async () => {
    const state = {
      accessToken: Math.random().toString(),
      identity: `user: ${Math.random()}`,
      accessTokenExpiry: Date.now() / 1000 + 1000,
    };
    setAuthStateCache(state);
    expect(await getAuthStateCache()).toEqual(state);
  });

  test('clear expired auth state when accessing synchronously', () => {
    const state = {
      accessToken: Math.random().toString(),
      identity: `user: ${Math.random()}`,
      accessTokenExpiry: Date.now() / 1000 - 1000,
    };
    setAuthStateCache(state);
    expect(getAuthStateCacheSync()).toBeNull();
  });

  test('clear expired auth state when accessing asynchronously', async () => {
    const state = {
      accessToken: Math.random().toString(),
      identity: `user: ${Math.random()}`,
      accessTokenExpiry: Date.now() / 1000 - 1000,
    };
    setAuthStateCache(state);
    expect(await getAuthStateCache()).toBeNull();
  });

  describe('msToExpire', () => {
    it('when no tokens', () => {
      const expireMs = msToExpire({
        identity: ANONYMOUS_IDENTITY,
      });
      expect(expireMs).toBe(Infinity);
    });

    it('when only access token', () => {
      const expireMs = msToExpire({
        identity: `user: ${Math.random()}`,
        accessToken: Math.random().toString(),
        accessTokenExpiry: Date.now() / 1000 + 1234,
      });
      expect(expireMs).toStrictEqual(1234000);
    });

    it('when only id token', () => {
      const expireMs = msToExpire({
        identity: `user: ${Math.random()}`,
        idToken: Math.random().toString(),
        idTokenExpiry: Date.now() / 1000 + 1234,
      });
      expect(expireMs).toStrictEqual(1234000);
    });

    it('old id token and new access token', () => {
      const expireMs = msToExpire({
        identity: `user: ${Math.random()}`,
        idToken: Math.random().toString(),
        idTokenExpiry: Date.now() / 1000 + 1234,
        accessToken: Math.random().toString(),
        accessTokenExpiry: Date.now() / 1000 + 4567,
      });
      expect(expireMs).toStrictEqual(1234000);
    });

    it('old access token and new id token', () => {
      const expireMs = msToExpire({
        identity: `user: ${Math.random()}`,
        idToken: Math.random().toString(),
        idTokenExpiry: Date.now() / 1000 + 4567,
        accessToken: Math.random().toString(),
        accessTokenExpiry: Date.now() / 1000 + 1234,
      });
      expect(expireMs).toStrictEqual(1234000);
    });
  });
});
