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

import { groupBy } from 'lodash-es';
import { reaction } from 'mobx';
import { addDisposer, flow, getEnv, getParentOfType, Instance, SnapshotIn, SnapshotOut, types } from 'mobx-state-tree';

import { LoadingState } from '../libs/constants';
import { BuilderID, BuildersService, ListBuildersResponse } from '../services/buildbucket';
import { AppState } from './app_state';

export const SearchPage = types
  .model('SearchPage', {
    searchQuery: '',
    loadedBuilders: types.array(types.frozen<BuilderID>()),
    loadingBuildersState: types.frozen<LoadingState>(LoadingState.Pending),
  })
  .views((self) => ({
    get builders() {
      return self.loadedBuilders.map<[string, BuilderID]>((b) => [
        `${b.project}/${b.bucket}/${b.builder}`.toLowerCase(),
        b,
      ]);
    },
    get filteredBuilders() {
      const searchQuery = self.searchQuery.toLowerCase();
      return this.builders?.filter(([bid]) => bid.includes(searchQuery)).map(([_, builder]) => builder);
    },
    get groupedBuilders() {
      return groupBy(this.filteredBuilders, (b) => `${b.project}/${b.bucket}`);
    },
  }))
  .actions((self) => ({
    setSearchQuery(newSearchString: string) {
      self.searchQuery = newSearchString;
    },
    loadAllBuilders: flow(function* (buildersService: BuildersService) {
      self.loadedBuilders.clear();
      self.loadingBuildersState = LoadingState.Running;

      let pageToken = '';
      try {
        do {
          const res: ListBuildersResponse = yield buildersService.listBuilders({ pageToken, pageSize: 1000 });
          self.loadingBuildersState = LoadingState.PartiallyFulfilled;
          self.loadedBuilders.push(...(res.builders?.map(({ id }) => id) || []));
          pageToken = res.nextPageToken || '';
        } while (pageToken);
      } catch (e) {
        self.loadingBuildersState = LoadingState.Rejected;
        return;
      }

      self.loadingBuildersState = LoadingState.Fulfilled;
    }),
    afterAttach: function () {
      addDisposer(
        self,
        reaction(
          () => getParentOfType(self, Store).appState.buildersService,
          (buildersService) => {
            if (!buildersService) {
              return;
            }
            this.loadAllBuilders(buildersService);
          },
          { fireImmediately: true }
        )
      );
    },
  }));

export interface StoreEnv {
  readonly appState: AppState;
}

export const Store = types
  .model({
    searchPage: SearchPage,
  })
  .views((self) => ({
    get appState() {
      return getEnv<StoreEnv>(self).appState as AppState;
    },
  }));

export type StoreInstance = Instance<typeof Store>;
export type StoreSnapshotIn = SnapshotIn<typeof Store>;
export type StoreSnapshotOut = SnapshotOut<typeof Store>;
