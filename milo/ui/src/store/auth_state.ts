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

import { reaction } from 'mobx';
import { addDisposer, Instance, SnapshotIn, SnapshotOut, types } from 'mobx-state-tree';

import { getAuthStateCache, setAuthStateCache } from '../auth_state_cache';
import { aliveFlow } from '../libs/milo_mobx_utils';
import { timeout } from '../libs/utils';
import { AuthState, queryAuthState } from '../services/milo_internal';

export const AuthStateStore = types
  .model('AuthStateStore', {
    id: types.optional(types.identifierNumber, () => Math.random()),
    value: types.maybe(types.frozen<AuthState>()),
  })
  .views((self) => ({
    get userIdentity() {
      return self.value?.identity;
    },
  }))
  .actions((self) => ({
    setValue(value: AuthState | null) {
      self.value = value || undefined;
    },
  }))
  .volatile(() => ({
    getAuthStateCache,
    setAuthStateCache,
    queryAuthState,
  }))
  .actions((self) => ({
    setDependencies(deps: Partial<Pick<typeof self, 'getAuthStateCache' | 'setAuthStateCache' | 'queryAuthState'>>) {
      Object.assign<typeof self, Partial<typeof self>>(self, deps);
    },
  }))
  .actions((self) => {
    // A unique reference that functions as the ID of the last
    // scheduleAuthStateUpdate call.
    let lastScheduleId = {};

    return {
      /**
       * Updates the auth state when before it expires. When called multiple times,
       * only the last call is respected.
       *
       * @param forceUpdate when set to true, update the auth state immediately.
       */
      scheduleUpdate: aliveFlow(self, function* (forceUpdate = false) {
        const scheduleId = {};
        lastScheduleId = scheduleId;
        const authState = self.value;

        let validDuration = 0;
        if (!forceUpdate && authState) {
          if (!authState.accessTokenExpiry) {
            return;
          }
          // Refresh the access token 10s earlier to prevent the token from
          // expiring before the new token is returned.
          validDuration = authState.accessTokenExpiry * 1000 - Date.now() - 10000;
        }

        yield timeout(validDuration);

        const call = self.queryAuthState();
        const newAuthState: Awaited<typeof call> = yield call;

        // There's another scheduled update. Abort the current one.
        if (lastScheduleId !== scheduleId) {
          return;
        }

        self.setAuthStateCache(newAuthState);
        self.setValue(newAuthState);
      }),
    };
  })
  .actions((self) => ({
    /**
     * Initialize the AuthStateStore and make it update its value when it
     * expires.
     */
    init: aliveFlow(self, function* () {
      try {
        const call = self.getAuthStateCache();
        const newAuthState: Awaited<typeof call> = yield call;

        // If there's an existing value, don't use the cache.
        // The `if` check after the getAuthStateCache call because there's a
        // slim chance that a new value is set after getAuthStateCache is called
        // and before the returned promise resolves.
        if (!self.value) {
          self.setValue(newAuthState);
        }
      } finally {
        let firstUpdate = true;
        addDisposer(
          self,
          reaction(
            () => self.value,
            () => {
              // Cookie could be updated when the page was offline. Update the
              // auth state immediately in the first update schedule.
              const wasFirstUpdate = firstUpdate;
              firstUpdate = false;
              self.scheduleUpdate(wasFirstUpdate);
            },
            {
              fireImmediately: true,
              // Ensure there are at least 10s between updates. So the backend
              // returning short-lived tokens won't cause the update action to
              // fire rapidly.
              // Note: the delay is not applied to the first call.
              delay: 10000,
            }
          )
        );
      }
    }),
  }));

export type AuthStateStoreInstance = Instance<typeof AuthStateStore>;
export type AuthStateStoreSnapshotIn = SnapshotIn<typeof AuthStateStore>;
export type AuthStateStoreSnapshotOut = SnapshotOut<typeof AuthStateStore>;
