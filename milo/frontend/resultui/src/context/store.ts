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

import { getEnv, Instance, SnapshotIn, SnapshotOut, types } from 'mobx-state-tree';

import { AppState } from './app_state';

export const SearchPage = types.model('SearchPage', { searchQuery: '' }).actions((self) => {
  const setSearchQuery = (newSearchString: string) => {
    self.searchQuery = newSearchString;
  };
  return { setSearchQuery };
});

export interface StoreEnv {
  readonly appState: AppState;
}

export const Store = types.model({ searchPage: SearchPage }).views((self) => ({
  get appState() {
    return getEnv<StoreEnv>(self).appState as AppState;
  },
}));

export type StoreInstance = Instance<typeof Store>;
export type StoreSnapshotIn = SnapshotIn<typeof Store>;
export type StoreSnapshotOut = SnapshotOut<typeof Store>;
