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

import { expect, jest } from '@jest/globals';
import { when } from 'mobx';
import { destroy, Instance, types } from 'mobx-state-tree';

import { ANONYMOUS_IDENTITY } from '@/common/libs/auth_state';
import { CacheOption } from '@/common/libs/cached_fn';
import { ListBuildersRequest } from '@/common/services/buildbucket';
import {
  QueryTestsRequest,
  QueryTestsResponse,
} from '@/common/services/luci_analysis';
import { ListBuildersResponse } from '@/common/services/milo_internal';
import { AuthStateStore } from '@/common/store/auth_state';
import { ServicesStore } from '@/common/store/services';

import { SearchPage, SearchTarget } from './search_page';

const listBuilderResponses: { [pageToken: string]: ListBuildersResponse } = {
  page1: {
    builders: [
      {
        id: { project: 'project1', bucket: 'bucket1', builder: 'builder1' },
        config: {},
      },
      {
        id: { project: 'project1', bucket: 'bucket1', builder: 'builder2' },
        config: {},
      },
      {
        id: { project: 'project1', bucket: 'bucket2', builder: 'builder1' },
        config: {},
      },
    ],
    nextPageToken: 'page2',
  },
  page2: {
    builders: [
      {
        id: { project: 'project1', bucket: 'bucket2', builder: 'builder2' },
        config: {},
      },
      {
        id: { project: 'project2', bucket: 'bucket1', builder: 'builder1' },
        config: {},
      },
      {
        id: { project: 'project2', bucket: 'bucket1', builder: 'builder2' },
        config: {},
      },
    ],
    nextPageToken: 'page3',
  },
  page3: {
    builders: [
      {
        id: { project: 'project2', bucket: 'bucket2', builder: 'builder1' },
        config: {},
      },
      {
        id: { project: 'project2', bucket: 'bucket2', builder: 'builder2' },
        config: {},
      },
    ],
  },
};

const TestStore = types.model('TestStore', {
  authState: types.optional(AuthStateStore, {
    id: 0,
    value: { identity: ANONYMOUS_IDENTITY },
  }),
  services: types.optional(ServicesStore, { id: 0, authState: 0 }),
  searchPage: types.optional(SearchPage, { services: 0 }),
});

describe('SearchPage', () => {
  describe('search builders', () => {
    let testStore: Instance<typeof TestStore>;
    let listBuildersStub: jest.SpiedFunction<
      (
        req: ListBuildersRequest,
        cacheOpt?: CacheOption
      ) => Promise<ListBuildersResponse>
    >;

    // The finish is signaled via a callback. It's easier to use `done` instead
    // of returning a promise.
    // eslint-disable-next-line jest/no-done-callback
    beforeEach((done) => {
      testStore = TestStore.create();
      listBuildersStub = jest.spyOn(testStore.services.milo!, 'listBuilders');

      listBuildersStub.mockImplementation(
        async ({ pageToken }) => listBuilderResponses[pageToken || 'page1']
      );
      testStore.searchPage.builderLoader?.loadRemainingPages();

      when(() => Boolean(testStore.searchPage.builderLoader?.loadedAll), done);
    });

    afterEach(() => destroy(testStore));

    it('should load builders correctly', () => {
      expect(listBuildersStub.mock.calls.length).toStrictEqual(3);
      expect(listBuildersStub.mock.calls[0][0]).toEqual({
        pageSize: 10000,
        pageToken: undefined,
      });
      expect(listBuildersStub.mock.calls[1][0]).toEqual({
        pageSize: 10000,
        pageToken: 'page2',
      });
      expect(listBuildersStub.mock.calls[2][0]).toEqual({
        pageSize: 10000,
        pageToken: 'page3',
      });
      expect(testStore.searchPage.builders).toEqual([
        [
          'project1/bucket1/builder1',
          { project: 'project1', bucket: 'bucket1', builder: 'builder1' },
        ],
        [
          'project1/bucket1/builder2',
          { project: 'project1', bucket: 'bucket1', builder: 'builder2' },
        ],
        [
          'project1/bucket2/builder1',
          { project: 'project1', bucket: 'bucket2', builder: 'builder1' },
        ],
        [
          'project1/bucket2/builder2',
          { project: 'project1', bucket: 'bucket2', builder: 'builder2' },
        ],
        [
          'project2/bucket1/builder1',
          { project: 'project2', bucket: 'bucket1', builder: 'builder1' },
        ],
        [
          'project2/bucket1/builder2',
          { project: 'project2', bucket: 'bucket1', builder: 'builder2' },
        ],
        [
          'project2/bucket2/builder1',
          { project: 'project2', bucket: 'bucket2', builder: 'builder1' },
        ],
        [
          'project2/bucket2/builder2',
          { project: 'project2', bucket: 'bucket2', builder: 'builder2' },
        ],
      ]);
    });

    it('should group builders correctly', () => {
      expect(testStore.searchPage.groupedBuilders).toEqual({
        'project1/bucket1': [
          { project: 'project1', bucket: 'bucket1', builder: 'builder1' },
          { project: 'project1', bucket: 'bucket1', builder: 'builder2' },
        ],
        'project1/bucket2': [
          { project: 'project1', bucket: 'bucket2', builder: 'builder1' },
          { project: 'project1', bucket: 'bucket2', builder: 'builder2' },
        ],
        'project2/bucket1': [
          { project: 'project2', bucket: 'bucket1', builder: 'builder1' },
          { project: 'project2', bucket: 'bucket1', builder: 'builder2' },
        ],
        'project2/bucket2': [
          { project: 'project2', bucket: 'bucket2', builder: 'builder1' },
          { project: 'project2', bucket: 'bucket2', builder: 'builder2' },
        ],
      });
    });

    it('should filter builders correctly', () => {
      testStore.searchPage.setSearchQuery('ject1/bucket2/bui');

      expect(testStore.searchPage.groupedBuilders).toEqual({
        'project1/bucket2': [
          { project: 'project1', bucket: 'bucket2', builder: 'builder1' },
          { project: 'project1', bucket: 'bucket2', builder: 'builder2' },
        ],
      });
    });

    it('should support fuzzy search', () => {
      testStore.searchPage.setSearchQuery('JECT1 Cket2');

      expect(testStore.searchPage.groupedBuilders).toEqual({
        'project1/bucket2': [
          { project: 'project1', bucket: 'bucket2', builder: 'builder1' },
          { project: 'project1', bucket: 'bucket2', builder: 'builder2' },
        ],
      });
    });
  });

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

    it('e2e', async () => {
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
