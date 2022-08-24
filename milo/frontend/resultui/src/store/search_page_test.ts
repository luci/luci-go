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
import { destroy, Instance } from 'mobx-state-tree';
import sinon from 'sinon';

import { CacheOption } from '../libs/cached_fn';
import { PrpcClientExt } from '../libs/prpc_client_ext';
import { deferred } from '../libs/utils';
import { BuildersService, ListBuildersRequest } from '../services/buildbucket';
import { ListBuildersResponse } from '../services/milo_internal';
import { QueryTestsRequest, QueryTestsResponse, TestHistoryService } from '../services/weetbix';
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

describe('SearchPage', () => {
  describe('search builders', () => {
    let searchPage: Instance<typeof SearchPage>;
    let listBuildersStub: sinon.SinonStub<
      [req: ListBuildersRequest, cacheOpt?: CacheOption | undefined],
      Promise<ListBuildersResponse>
    >;

    beforeEach((done) => {
      const buildersService = new BuildersService(new PrpcClientExt({}, () => ''));
      listBuildersStub = sinon.stub(buildersService, 'listBuilders');

      listBuildersStub.callsFake(async ({ pageToken }) => listBuilderResponses[pageToken || 'page1']);
      searchPage = SearchPage.create();
      searchPage.setDependencies(buildersService, null);

      when(() => Boolean(searchPage.builderLoader?.loadedAll), done);
    });

    afterEach(() => destroy(searchPage));

    it('should load builders correctly', () => {
      expect(listBuildersStub.callCount).to.eq(3);
      expect(listBuildersStub.getCall(0).args[0]).to.deep.eq({ pageSize: 1000, pageToken: undefined });
      expect(listBuildersStub.getCall(1).args[0]).to.deep.eq({ pageSize: 1000, pageToken: 'page2' });
      expect(listBuildersStub.getCall(2).args[0]).to.deep.eq({ pageSize: 1000, pageToken: 'page3' });
      expect(searchPage.builders).to.deep.eq([
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
      expect(searchPage.groupedBuilders).to.deep.equal({
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
      searchPage.setSearchQuery('ject1/bucket2/bui');

      expect(searchPage.groupedBuilders).to.deep.equal({
        'project1/bucket2': [
          { project: 'project1', bucket: 'bucket2', builder: 'builder1' },
          { project: 'project1', bucket: 'bucket2', builder: 'builder2' },
        ],
      });
    });
  });

  describe('search tests', () => {
    let searchPage: Instance<typeof SearchPage>;
    let queryTestsStub: sinon.SinonStub<
      [req: QueryTestsRequest, cacheOpt?: CacheOption | undefined],
      Promise<QueryTestsResponse>
    >;

    beforeEach(() => {
      const testHistoryService = new TestHistoryService(new PrpcClientExt({}, () => ''));
      queryTestsStub = sinon.stub(testHistoryService, 'queryTests');

      searchPage = SearchPage.create();
      searchPage.setDependencies(null, testHistoryService);
    });

    afterEach(() => destroy(searchPage));

    it('e2e', async () => {
      searchPage.setSearchTarget(SearchTarget.Tests);
      searchPage.setTestProject('testProject');
      expect(queryTestsStub.callCount).to.eq(0);

      // The first page should be loaded automatically when the searchQuery is
      // set.
      const [promise1, resolve1] = deferred();
      queryTestsStub.onCall(0).callsFake(async () => {
        resolve1();
        return { testIds: ['testId1substr1', 'testId1substr2'], nextPageToken: 'page2' };
      });
      searchPage.setSearchQuery('substr');
      await promise1;
      expect(queryTestsStub.callCount).to.eq(1);
      expect(queryTestsStub.getCall(0).args[0]).to.deep.eq({
        project: 'testProject',
        testIdSubstring: 'substr',
        pageToken: undefined,
      });

      // The subsequent page loads should be handled correctly.
      queryTestsStub.onCall(1).resolves({ testIds: ['testId1substr3', 'testId1substr4'], nextPageToken: 'page3' });
      queryTestsStub.onCall(2).resolves({ testIds: ['testId1substr3', 'testId1substr4'] });
      await searchPage.testLoader?.loadNextPage();
      await searchPage.testLoader?.loadNextPage();
      expect(queryTestsStub.callCount).to.eq(3);
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
      const [promise2, resolve2] = deferred();
      queryTestsStub.onCall(3).callsFake(async () => {
        resolve2();
        return { testIds: ['testId1substr1'] };
      });
      searchPage.setSearchQuery('substr1');
      await promise2;

      expect(queryTestsStub.callCount).to.eq(4);
      expect(queryTestsStub.getCall(3).args[0]).to.deep.eq({
        project: 'testProject',
        testIdSubstring: 'substr1',
        pageToken: undefined,
      });
    });
  });
});
