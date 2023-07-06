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

import { computed } from 'mobx';
import { addDisposer, types } from 'mobx-state-tree';
import { keepAlive } from 'mobx-utils';

import { PageLoader } from '@/common/models/page_loader';
import { ServicesStore } from '@/common/store/services';

export enum SearchTarget {
  Builders = 'BUILDERS',
  Tests = 'TESTS',
}

export const DEFAULT_SEARCH_TARGET = SearchTarget.Builders;
export const DEFAULT_TEST_PROJECT = 'chromium';

export const SearchPage = types
  .model('SearchPage', {
    services: types.safeReference(ServicesStore),

    searchTarget: types.frozen<SearchTarget>(DEFAULT_SEARCH_TARGET),
    searchQuery: '',

    testProject: DEFAULT_TEST_PROJECT,
  })
  .views((self) => ({
    get testLoader() {
      const testHistoryService = self.services?.testHistory;
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
    setDependencies(deps: Pick<typeof self, 'services'>) {
      Object.assign(self, deps);
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
      addDisposer(self, keepAlive(computed(() => self.testLoader)));
    },
  }));
