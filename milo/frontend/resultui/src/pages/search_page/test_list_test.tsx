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

import { render } from '@testing-library/react';
import { expect } from 'chai';
import { destroy } from 'mobx-state-tree';
import * as sinon from 'sinon';

import { ANONYMOUS_IDENTITY } from '../../services/milo_internal';
import { Store, StoreContext } from '../../store';
import { SearchTarget } from '../../store/search_page';
import { TestList } from './test_list';

describe('TestList', () => {
  let timer: sinon.SinonFakeTimers;
  beforeEach(() => {
    timer = sinon.useFakeTimers();
  });
  afterEach(() => timer.restore());

  it('should load the first page of test', () => {
    const store = Store.create({ authState: { identity: ANONYMOUS_IDENTITY } });
    store.searchPage.setSearchQuery('test-id');
    store.searchPage.setSearchTarget(SearchTarget.Tests);
    after(() => destroy(store));
    const loadFirstPagesStub = sinon.stub(store.searchPage.testLoader!, 'loadFirstPage');

    render(
      <StoreContext.Provider value={store}>
        <TestList />
      </StoreContext.Provider>
    );

    timer.runAll();

    expect(loadFirstPagesStub.callCount).to.eq(1);
  });
});
