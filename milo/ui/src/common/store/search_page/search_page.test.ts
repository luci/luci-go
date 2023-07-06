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

import { destroy, Instance, types } from 'mobx-state-tree';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import {
  QueryTestsRequest,
  QueryTestsResponse,
} from '@/common/services/luci_analysis';
import { AuthStateStore } from '@/common/store/auth_state';
import { ServicesStore } from '@/common/store/services';
import { CacheOption } from '@/generic_libs/tools/cached_fn';

import { SearchPage, SearchTarget } from './search_page';

const TestStore = types.model('TestStore', {
  authState: types.optional(AuthStateStore, {
    id: 0,
    value: { identity: ANONYMOUS_IDENTITY },
  }),
  services: types.optional(ServicesStore, { id: 0, authState: 0 }),
  searchPage: types.optional(SearchPage, { services: 0 }),
});

describe('SearchPage', () => {
  describe('search tests', () => {
    let testStore: Instance<typeof TestStore>;
    let queryTestsStub: jest.SpiedFunction<
      (
        req: QueryTestsRequest,
        cacheOpt?: CacheOption
      ) => Promise<QueryTestsResponse>
    >;

    beforeEach(() => {
      testStore = TestStore.create();
      queryTestsStub = jest.spyOn(
        testStore.services.testHistory!,
        'queryTests'
      );
    });

    afterEach(() => destroy(testStore));

    test('e2e', async () => {
      testStore.searchPage.setSearchTarget(SearchTarget.Tests);
      testStore.searchPage.setTestProject('testProject');
      testStore.searchPage.setSearchQuery('substr');
      expect(queryTestsStub.mock.calls.length).toStrictEqual(0);

      // The subsequent page loads should be handled correctly.
      queryTestsStub.mockResolvedValueOnce({
        testIds: ['testId1substr1', 'testId1substr2'],
        nextPageToken: 'page2',
      });
      queryTestsStub.mockResolvedValueOnce({
        testIds: ['testId1substr3', 'testId1substr4'],
        nextPageToken: 'page3',
      });
      queryTestsStub.mockResolvedValueOnce({
        testIds: ['testId1substr3', 'testId1substr4'],
      });
      await testStore.searchPage.testLoader?.loadNextPage();
      await testStore.searchPage.testLoader?.loadNextPage();
      await testStore.searchPage.testLoader?.loadNextPage();
      expect(queryTestsStub.mock.calls.length).toStrictEqual(3);
      expect(queryTestsStub.mock.calls[0][0]).toEqual({
        project: 'testProject',
        testIdSubstring: 'substr',
        pageToken: undefined,
      });
      expect(queryTestsStub.mock.calls[1][0]).toEqual({
        project: 'testProject',
        testIdSubstring: 'substr',
        pageToken: 'page2',
      });
      expect(queryTestsStub.mock.calls[2][0]).toEqual({
        project: 'testProject',
        testIdSubstring: 'substr',
        pageToken: 'page3',
      });

      // When the search query is reset, trigger a fresh page load with the new
      // query.
      queryTestsStub.mockImplementationOnce(async () => {
        return { testIds: ['testId1substr1'] };
      });
      testStore.searchPage.setSearchQuery('substr1');
      await testStore.searchPage.testLoader?.loadNextPage();

      expect(queryTestsStub.mock.calls.length).toStrictEqual(4);
      expect(queryTestsStub.mock.calls[3][0]).toEqual({
        project: 'testProject',
        testIdSubstring: 'substr1',
        pageToken: undefined,
      });
    });
  });
});
