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
import { Instance } from 'mobx-state-tree';

import { SearchPage } from './store';

describe('SearchPage', () => {
  let searchPage: Instance<typeof SearchPage>;
  beforeEach(() => {
    searchPage = SearchPage.create({
      loadedBuilders: [
        { project: 'project1', bucket: 'bucket1', builder: 'builder1' },
        { project: 'project1', bucket: 'bucket1', builder: 'builder2' },
        { project: 'project1', bucket: 'bucket2', builder: 'builder1' },
        { project: 'project1', bucket: 'bucket2', builder: 'builder2' },
        { project: 'project2', bucket: 'bucket1', builder: 'builder1' },
        { project: 'project2', bucket: 'bucket1', builder: 'builder2' },
        { project: 'project2', bucket: 'bucket2', builder: 'builder1' },
        { project: 'project2', bucket: 'bucket2', builder: 'builder2' },
      ],
    });
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
