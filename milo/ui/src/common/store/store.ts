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
import { createContext, useContext } from 'react';

import { createContextLink } from '@/generic_libs/tools/lit_context';

import { AuthStateStore } from './auth_state';
import { BuildPage } from './build_page';
import { InvocationPage } from './invocation_page';
import { ServicesStore } from './services';
import { TestHistoryPage } from './test_history_page';
import { Timestamp } from './timestamp';
import { UserConfig } from './user_config';

export const Store = types
  .model('Store', {
    currentTime: types.optional(Timestamp, {}),
    refreshTime: types.optional(Timestamp, {}),

    authState: types.optional(AuthStateStore, {}),
    userConfig: types.optional(UserConfig, {}),
    services: types.optional(ServicesStore, {}),

    buildPage: types.optional(BuildPage, {}),
    testHistoryPage: types.optional(TestHistoryPage, {}),
    invocationPage: types.optional(InvocationPage, {}),
  })
  .volatile(() => ({
    /**
     * The service worker that performs redirection.
     *
     * undefined means it's not initialized yet.
     * null means there's no such service worker.
     */
    redirectSw: undefined as ServiceWorkerRegistration | null | undefined,
  }))
  .actions((self) => ({
    setRedirectSw(redirectSw: ServiceWorkerRegistration | null) {
      self.redirectSw = redirectSw;
    },
    afterCreate() {
      self.services.setDependencies({ authState: self.authState });
      self.userConfig.enableCaching();
      self.buildPage.setDependencies({
        currentTime: self.currentTime,
        services: self.services,
        userConfig: self.userConfig,
      });
      self.testHistoryPage.setDependencies({
        refreshTime: self.refreshTime,
        services: self.services,
      });
      self.invocationPage.setDependencies({
        services: self.services,
      });
    },
  }));

export type StoreInstance = Instance<typeof Store>;
export type StoreSnapshotIn = SnapshotIn<typeof Store>;
export type StoreSnapshotOut = SnapshotOut<typeof Store>;

export const [provideStore, consumeStore] = createContextLink<StoreInstance>();

export const StoreContext = createContext<StoreInstance | null>(null);
export const StoreProvider = StoreContext.Provider;

export function useStore() {
  const context = useContext(StoreContext);
  if (!context) {
    throw new Error('useStore must be used within StoreProvider');
  }

  return context;
}
