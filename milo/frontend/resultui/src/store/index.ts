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

import { AppState } from '../context/app_state';
import { SearchPage } from './search_page';

export const Store = types
  .model({
    appState: types.maybe(types.frozen<AppState>()),
    searchPage: types.optional(SearchPage, {}),
  })
  .actions((self) => ({
    setAppState(appState: AppState | null) {
      self.appState = appState ?? undefined;
    },
    afterCreate() {
      addDisposer(
        self,
        reaction(
          () => [self.appState?.buildersService, self.appState?.testHistoryService] as const,
          ([buildersService, testHistoryService]) => {
            self.searchPage.setDependencies(buildersService || null, testHistoryService || null);
          },
          { fireImmediately: true }
        )
      );
    },
  }));

export type StoreInstance = Instance<typeof Store>;
export type StoreSnapshotIn = SnapshotIn<typeof Store>;
export type StoreSnapshotOut = SnapshotOut<typeof Store>;
