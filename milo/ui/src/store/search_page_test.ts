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

import { expect } from 'chai';
import { when } from 'mobx';
import { destroy, Instance, types } from 'mobx-state-tree';
import sinon from 'sinon';

import { ANONYMOUS_IDENTITY } from '../libs/auth_state';
import { CacheOption } from '../libs/cached_fn';
import { ListBuildersRequest } from '../services/buildbucket';
import { QueryTestsRequest, QueryTestsResponse } from '../services/luci_analysis';
import { ListBuildersResponse } from '../services/milo_internal';
import { AuthStateStore } from './auth_state';
import { SearchPage, SearchTarget } from './search_page';
import { ServicesStore } from './services';

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
  authState: types.optional(AuthStateStore, { id: 0, value: { identity: ANONYMOUS_IDENTITY } }),
  services: types.optional(ServicesStore, { id: 0, authState: 0 }),
  searchPage: types.optional(SearchPage, { services: 0 }),
});

describe('SearchPage', () => {
  describe('search builders', () => {
    let testStore: Instance<typeof TestStore>;
    let listBuildersStub: sinon.SinonStub<
      [req: ListBuildersRequest, cacheOpt?: CacheOption | undefined],
      Promise<ListBuildersResponse>
    >;

    beforeEach((done) => {
      testStore = TestStore.create();
      listBuildersStub = sinon.stub(testStore.services.milo!, 'listBuilders');

      listBuildersStub.callsFake(async ({ pageToken }) => listBuilderResponses[pageToken || 'page1']);
      testStore.searchPage.builderLoader?.loadRemainingPages();

      when(() => Boolean(testStore.searchPage.builderLoader?.loadedAll), done);
    });

    afterEach(() => destroy(testStore));

    it('should load builders correctly', () => {
      expect(listBuildersStub.callCount).to.eq(3);
      expect(listBuildersStub.getCall(0).args[0]).to.deep.eq({ pageSize: 10000, pageToken: undefined });
      expect(listBuildersStub.getCall(1).args[0]).to.deep.eq({ pageSize: 10000, pageToken: 'page2' });
      expect(listBuildersStub.getCall(2).args[0]).to.deep.eq({ pageSize: 10000, pageToken: 'page3' });
      expect(testStore.searchPage.builders).to.deep.eq([
        ['project1/bucket1/builder1', { project: 'project1', bucket: 'bucket1', builder: 'builder1' }],
        ['project1/bucket1/builder2', { project: 'project1', bucket: 'bucket1', builder: 'builder2' }],
        ['project1/bucket2/builder1', { project: 'project1', bucket: 'bucket2', builder: 'builder1' }],
        ['project1/bucket2/builder2', { project: 'project1', bucket: 'bucket2', builder: 'builder2' }],
        ['project2/bucket1/builder1', { project: 'project2', bucket: 'bucket1', builder: 'builder1' }],
        ['project2/bucket1/builder2', { project: 'project2', bucket: 'bucket1', builder: 'builder2' }],
        ['project2/bucket2/builder1', { project: 'project2', bucket: 'bucket2', builder: 'builder1' }],
        ['project2/bucket2/builder2', { project: 'project2', bucket: 'bucket2', builder: 'builder2' }],
      ]);
    });

    it('should group builders correctly', () => {
      expect(testStore.searchPage.groupedBuilders).to.deep.equal({
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

      expect(testStore.searchPage.groupedBuilders).to.deep.equal({
        'project1/bucket2': [
          { project: 'project1', bucket: 'bucket2', builder: 'builder1' },
          { project: 'project1', bucket: 'bucket2', builder: 'builder2' },
        ],
      });
    });

    it('should support fuzzy search', () => {
      testStore.searchPage.setSearchQuery('JECT1 Cket2');

      expect(testStore.searchPage.groupedBuilders).to.deep.equal({
        'project1/bucket2': [
          { project: 'project1', bucket: 'bucket2', builder: 'builder1' },
          { project: 'project1', bucket: 'bucket2', builder: 'builder2' },
        ],
      });
    });
  });

  describe('search tests', () => {
    let testStore: Instance<typeof TestStore>;
    let queryTestsStub: sinon.SinonStub<
      [req: QueryTestsRequest, cacheOpt?: CacheOption | undefined],
      Promise<QueryTestsResponse>
    >;

    beforeEach(() => {
      testStore = TestStore.create();
      queryTestsStub = sinon.stub(testStore.services.testHistory!, 'queryTests');
    });

    afterEach(() => destroy(testStore));

    it('e2e', async () => {
      testStore.searchPage.setSearchTarget(SearchTarget.Tests);
      testStore.searchPage.setTestProject('testProject');
      testStore.searchPage.setSearchQuery('substr');
      expect(queryTestsStub.callCount).to.eq(0);

      // The subsequent page loads should be handled correctly.
      queryTestsStub.onCall(0).resolves({ testIds: ['testId1substr1', 'testId1substr2'], nextPageToken: 'page2' });
      queryTestsStub.onCall(1).resolves({ testIds: ['testId1substr3', 'testId1substr4'], nextPageToken: 'page3' });
      queryTestsStub.onCall(2).resolves({ testIds: ['testId1substr3', 'testId1substr4'] });
      await testStore.searchPage.testLoader?.loadNextPage();
      await testStore.searchPage.testLoader?.loadNextPage();
      await testStore.searchPage.testLoader?.loadNextPage();
      expect(queryTestsStub.callCount).to.eq(3);
      expect(queryTestsStub.getCall(0).args[0]).to.deep.eq({
        project: 'testProject',
        testIdSubstring: 'substr',
        pageToken: undefined,
      });
      expect(queryTestsStub.getCall(1).args[0]).to.deep.eq({
        project: 'testProject',
        testIdSubstring: 'substr',
        pageToken: 'page2',
      });
      expect(queryTestsStub.getCall(2).args[0]).to.deep.eq({
        project: 'testProject',
        testIdSubstring: 'substr',
        pageToken: 'page3',
      });

      // When the search query is reset, trigger a fresh page load with the new
      // query.
      queryTestsStub.onCall(3).callsFake(async () => {
        return { testIds: ['testId1substr1'] };
      });
      testStore.searchPage.setSearchQuery('substr1');
      await testStore.searchPage.testLoader?.loadNextPage();

      expect(queryTestsStub.callCount).to.eq(4);
      expect(queryTestsStub.getCall(3).args[0]).to.deep.eq({
        project: 'testProject',
        testIdSubstring: 'substr1',
        pageToken: undefined,
      });
    });
  });
});
