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
import {
  addDisposer,
  Instance,
  SnapshotIn,
  SnapshotOut,
  types,
} from 'mobx-state-tree';

import {
  AuthState,
  msToExpire,
  queryAuthState,
  setAuthStateCache,
} from '@/common/api/auth_state';
import { aliveFlow } from '@/generic_libs/tools/mobx_utils';
import { timeout } from '@/generic_libs/tools/utils';

export const AuthStateStore = types
  .model('AuthStateStore', {
    id: types.optional(types.identifierNumber, () => Math.random()),
    /**
     * `undefined` means the auth state is not yet initialized. (i.e. we don't
     * know whether the user is signed in or not.)
     * Once the auth state is initialized, it will remain that way.
     */
    value: types.maybe(types.frozen<AuthState>()),
  })
  // The following properties should be used in favor of `value` (e.g.
  // `value.identity`) to avoid triggering unnecessary updates when auth state
  // is refreshed.
  .views((self) => ({
    get identity() {
      return self.value?.identity;
    },
    get email() {
      return self.value?.email;
    },
    get picture() {
      return self.value?.picture;
    },
  }))
  .volatile(() => ({
    setAuthStateCache,
    queryAuthState,
  }))
  .actions((self) => ({
    setDependencies(
      deps: Partial<Pick<typeof self, 'setAuthStateCache' | 'queryAuthState'>>
    ) {
      Object.assign<typeof self, Partial<typeof self>>(self, deps);
    },
  }))
  .actions((self) => {
    // A unique reference that functions as the ID of the last scheduleUpdate
    // call.
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

        const validDuration =
          forceUpdate || !authState
            ? 0
            : // Refresh the auth state 10s earlier to prevent the tokens from
              // expiring before the new tokens are returned.
              msToExpire(authState) - 10000;
        if (validDuration === Infinity) {
          return;
        }

        yield timeout(validDuration);

        const call = self.queryAuthState();
        const newAuthState: Awaited<typeof call> = yield call;

        // There's another scheduled update. Abort the current one.
        if (lastScheduleId !== scheduleId) {
          return;
        }

        self.setAuthStateCache(newAuthState);
        self.value = newAuthState;
      }),
    };
  })
  .actions((self) => ({
    /**
     * Initialize the AuthStateStore and make it update its value when it
     * expires.
     */
    init(initialValue: AuthState) {
      self.value = initialValue;
      let isFirstUpdate = true;
      addDisposer(
        self,
        reaction(
          () => self.value,
          () => {
            // The initial value could be outdated. Force update when first
            // initialized.
            self.scheduleUpdate(isFirstUpdate);
            isFirstUpdate = false;
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
    },
  }));

export type AuthStateStoreInstance = Instance<typeof AuthStateStore>;
export type AuthStateStoreSnapshotIn = SnapshotIn<typeof AuthStateStore>;
export type AuthStateStoreSnapshotOut = SnapshotOut<typeof AuthStateStore>;
