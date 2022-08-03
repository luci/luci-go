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
import { addDisposer, getEnv, getParentOfType, Instance, SnapshotIn, SnapshotOut, types } from 'mobx-state-tree';

import { PageLoader } from '../libs/page_loader';
import { BuilderID, BuildersService } from '../services/buildbucket';
import { TestHistoryService } from '../services/weetbix';
import { AppState } from './app_state';

export const enum SearchTarget {
  Builders,
  Tests,
}

export const SearchPage = types
  .model('SearchPage', {
    testHistoryService: types.frozen<TestHistoryService | null>(null),
    buildersService: types.frozen<BuildersService | null>(null),

    searchTarget: types.frozen<SearchTarget>(SearchTarget.Builders),
    searchQuery: '',

    testProject: 'chromium',
  })
  .views((self) => ({
    get builderLoader() {
      const buildersService = self.buildersService;
      if (!buildersService) {
        return null;
      }

      return new PageLoader<BuilderID>(async (pageToken) => {
        const res = await buildersService.listBuilders({
          pageToken,
          pageSize: 1000,
        });
        return [res.builders?.map((b) => b.id) || [], res.nextPageToken];
      });
    },
    get builders() {
      return this.builderLoader?.items.map<[string, BuilderID]>((b) => [
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
    get testLoader() {
      const testHistoryService = self.testHistoryService;
      const project = self.testProject;
      const testIdSubstring = self.searchQuery;
      if (!testHistoryService || !project || !testIdSubstring) {
        return null;
      }

      return new PageLoader<string>(async (pageToken) => {
        const res = await testHistoryService.queryTests({
          project,
          testIdSubstring,
          pageToken,
        });
        return [res.testIds || [], res.nextPageToken];
      });
    },
  }))
  .actions((self) => ({
    setDependencies(buildersService: BuildersService | null, testHistoryService: TestHistoryService | null) {
      self.buildersService = buildersService;
      self.testHistoryService = testHistoryService;
    },
    setSearchTarget(newSearchTarget: SearchTarget) {
      self.searchTarget = newSearchTarget;
    },
    setSearchQuery(newSearchString: string) {
      self.searchQuery = newSearchString;
    },
    setTestProject(newTestProject: string) {
      self.testProject = newTestProject;
    },
    afterCreate: function () {
      addDisposer(
        self,
        reaction(
          () => self.builderLoader,
          async (builderLoader) => {
            if (!builderLoader) {
              return;
            }
            while (!builderLoader.loadedAll) {
              await builderLoader.loadNextPage();
            }
          },
          { fireImmediately: true }
        )
      );

      addDisposer(
        self,
        reaction(
          () => [self.searchTarget, self.testLoader] as const,
          ([searchTarget, testLoader]) => {
            if (searchTarget !== SearchTarget.Tests) {
              return;
            }
            testLoader?.loadFirstPage();
          },
          { fireImmediately: true }
        )
      );
    },
    afterAttach() {
      addDisposer(
        self,
        reaction(
          () => {
            const appState: AppState = getParentOfType(self, Store).appState;
            return [appState.buildersService, appState.testHistoryService] as const;
          },
          (deps) => this.setDependencies(...deps),
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
