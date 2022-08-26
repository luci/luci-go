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

import { PrpcClientOptions, RpcCode } from '@chopsui/prpc-client';
import stableStringify from 'fast-json-stable-stringify';
import { computed, reaction, untracked } from 'mobx';
import { addDisposer, Instance, SnapshotIn, SnapshotOut, types } from 'mobx-state-tree';
import { keepAlive } from 'mobx-utils';
import { createContext, useContext } from 'react';

import { MAY_REQUIRE_SIGNIN } from '../common_tags';
import { createContextLink } from '../libs/context';
import { PrpcClientExt } from '../libs/prpc_client_ext';
import { attachTags } from '../libs/tag';
import { BuilderID, BuildersService, BuildsService } from '../services/buildbucket';
import { AuthState, MiloInternal } from '../services/milo_internal';
import { ResultDb } from '../services/resultdb';
import { ClustersService, TestHistoryService } from '../services/weetbix';
import { SearchPage } from './search_page';
import { UserConfig } from './user_config';

const MAY_REQUIRE_SIGNIN_ERROR_CODE = [RpcCode.NOT_FOUND, RpcCode.PERMISSION_DENIED, RpcCode.UNAUTHENTICATED];

export const Store = types
  .model({
    timestamp: Date.now(),
    selectedTabId: types.maybe(types.string),
    // Use number instead of boolean because previousPage.disconnectedCallback
    // might be called after currentPage.disconnectedCallback.
    // Number allows us to reset the setting even when the execution is out of
    // order.
    /**
     * When hasSettingsDialog > 0, the current page has a settings dialog.
     *
     * If the page has a setting dialog, it should increment the number on connect
     * and decrement it on disconnect.
     */
    hasSettingsDialog: 0,
    showSettingsDialog: false,
    authState: types.maybe(types.frozen<AuthState>()),
    banners: types.array(types.frozen<unknown>()),
    userConfig: types.optional(UserConfig, {}),
    searchPage: types.optional(SearchPage, {}),
  })
  .volatile(() => {
    const cachedBuildId = new Map<string, string>();
    return {
      setBuildId(builderId: BuilderID, buildNum: number, buildId: string) {
        cachedBuildId.set(stableStringify([builderId, buildNum]), buildId);
      },
      getBuildId(builderId: BuilderID, buildNum: number) {
        return cachedBuildId.get(stableStringify([builderId, buildNum]));
      },
      /**
       * undefined means it's not initialized yet.
       * null means there's no such service worker.
       */
      redirectSw: undefined as ServiceWorkerRegistration | null | undefined,
    };
  })
  .views((self) => {
    function makeClient(opts: PrpcClientOptions) {
      // Don't track the access token so services won't be refreshed when the
      // access token is updated.
      return new PrpcClientExt(
        opts,
        () => untracked(() => self.authState?.accessToken || ''),
        (e) => {
          if (MAY_REQUIRE_SIGNIN_ERROR_CODE.includes(e.code)) {
            attachTags(e, MAY_REQUIRE_SIGNIN);
          }
          throw e;
        }
      );
    }

    return {
      get userIdentity() {
        return self.authState?.identity;
      },
      get resultDb() {
        if (!this.userIdentity) {
          return null;
        }
        return new ResultDb(makeClient({ host: CONFIGS.RESULT_DB.HOST }));
      },
      get testHistoryService() {
        if (!this.userIdentity) {
          return null;
        }
        return new TestHistoryService(makeClient({ host: CONFIGS.WEETBIX.HOST }));
      },
      get milo() {
        if (!this.userIdentity) {
          return null;
        }
        return new MiloInternal(makeClient({ host: '' }));
      },
      get buildsService() {
        if (!this.userIdentity) {
          return null;
        }
        return new BuildsService(makeClient({ host: CONFIGS.BUILDBUCKET.HOST }));
      },
      get buildersService() {
        if (!this.userIdentity) {
          return null;
        }
        return new BuildersService(makeClient({ host: CONFIGS.BUILDBUCKET.HOST }));
      },
      get clustersService() {
        if (!this.userIdentity) {
          return null;
        }
        return new ClustersService(makeClient({ host: CONFIGS.WEETBIX.HOST }));
      },
    };
  })
  .actions((self) => ({
    setAuthState(authState: AuthState | null) {
      self.authState = authState || undefined;
    },
    setRedirectSw(redirectSw: ServiceWorkerRegistration | null) {
      self.redirectSw = redirectSw;
    },
    refresh() {
      self.timestamp = Date.now();
    },
    setSelectedTabId(tabId: string) {
      self.selectedTabId = tabId;
    },
    registerSettingsDialog() {
      self.hasSettingsDialog++;
    },
    unregisterSettingsDialog() {
      self.hasSettingsDialog--;
    },
    addBanner(banner: unknown) {
      self.banners.push(banner);
    },
    removeBanner(banner: unknown) {
      self.banners.remove(banner);
    },
    setShowSettingsDialog(show: boolean) {
      self.showSettingsDialog = show;
    },
    afterCreate() {
      addDisposer(
        self,
        reaction(
          () => [self.buildersService, self.testHistoryService] as const,
          ([buildersService, testHistoryService]) => {
            self.searchPage.setDependencies(buildersService || null, testHistoryService || null);
          },
          { fireImmediately: true }
        )
      );

      // These computed properties contains internal caches. Keep them alive.
      addDisposer(self, keepAlive(computed(() => self.resultDb)));
      addDisposer(self, keepAlive(computed(() => self.testHistoryService)));
      addDisposer(self, keepAlive(computed(() => self.milo)));
      addDisposer(self, keepAlive(computed(() => self.buildsService)));
      addDisposer(self, keepAlive(computed(() => self.buildersService)));
      addDisposer(self, keepAlive(computed(() => self.clustersService)));
    },
  }));

export type StoreInstance = Instance<typeof Store>;
export type StoreSnapshotIn = SnapshotIn<typeof Store>;
export type StoreSnapshotOut = SnapshotOut<typeof Store>;

export const [provideStore, consumeStore] = createContextLink<StoreInstance>();

export const StoreContext = createContext<StoreInstance | null>(null);

export function useStore() {
  const context = useContext(StoreContext);
  if (!context) {
    throw new Error('useStore must be used within StoreProvider');
  }

  return context;
}
