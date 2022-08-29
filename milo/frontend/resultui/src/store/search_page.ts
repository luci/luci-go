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
import { computed } from 'mobx';
import { addDisposer, types } from 'mobx-state-tree';
import { keepAlive } from 'mobx-utils';

import { PageLoader } from '../libs/page_loader';
import { BuilderID, BuildersService } from '../services/buildbucket';
import { TestHistoryService } from '../services/weetbix';

export const enum SearchTarget {
  Builders,
  Tests,
}

export const SearchPage = types
  .model('SearchPage', {
    searchTarget: types.frozen<SearchTarget>(SearchTarget.Builders),
    searchQuery: '',

    testProject: 'chromium',
  })
  .volatile(() => ({
    testHistoryService: null as TestHistoryService | null,
    buildersService: null as BuildersService | null,
  }))
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
    afterCreate() {
      // These computed properties contains internal caches. Keep them alive.
      addDisposer(self, keepAlive(computed(() => self.builderLoader)));
      addDisposer(self, keepAlive(computed(() => self.testLoader)));
    },
  }));
