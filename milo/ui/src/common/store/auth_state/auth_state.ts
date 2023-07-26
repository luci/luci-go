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

import { Instance, SnapshotIn, SnapshotOut, types } from 'mobx-state-tree';

import { AuthState } from '@/common/api/auth_state';

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
  .actions((self) => ({
    setValue(value: AuthState) {
      self.value = value;
    },
  }));

export type AuthStateStoreInstance = Instance<typeof AuthStateStore>;
export type AuthStateStoreSnapshotIn = SnapshotIn<typeof AuthStateStore>;
export type AuthStateStoreSnapshotOut = SnapshotOut<typeof AuthStateStore>;
