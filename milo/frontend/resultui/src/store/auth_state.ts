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

import { AuthState } from '../services/milo_internal';

export const AuthStateStore = types
  .model('AuthStateStore', {
    id: types.optional(types.identifier, () => `AuthStateStore/${Math.random()}`),
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
  }));

export type AuthStateStoreInstance = Instance<typeof AuthStateStore>;
export type AuthStateStoreSnapshotIn = SnapshotIn<typeof AuthStateStore>;
export type AuthStateStoreSnapshotOut = SnapshotOut<typeof AuthStateStore>;
