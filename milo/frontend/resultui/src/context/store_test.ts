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
import { Instance } from 'mobx-state-tree';
import sinon, { SinonMock } from 'sinon';

import { PrpcClientExt } from '../libs/prpc_client_ext';
import { BuildersService } from '../services/buildbucket';
import { ListBuildersResponse } from '../services/milo_internal';
import { SearchPage } from './store';

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
  let searchPage: Instance<typeof SearchPage>;
  let mock: SinonMock;
  beforeEach((done) => {
    const buildersService = new BuildersService(new PrpcClientExt({}, () => ''));
    mock = sinon.mock(buildersService);

    mock
      .expects('listBuilders')
      .exactly(3)
      .callsFake(async ({ pageToken }) => listBuilderResponses[pageToken || 'page1']);
    searchPage = SearchPage.create({ buildersService });

    when(() => Boolean(searchPage.builderLoader?.loadedAll), done);
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
