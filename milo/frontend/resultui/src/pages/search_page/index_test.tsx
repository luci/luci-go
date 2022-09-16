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

import { fireEvent, render, screen } from '@testing-library/react';
import { expect } from 'chai';
import { destroy, protect, unprotect } from 'mobx-state-tree';
import * as sinon from 'sinon';

import { URLExt } from '../../libs/utils';
import { Store, StoreProvider } from '../../store';
import { SearchTarget } from '../../store/search_page';
import { SearchPage } from '.';

describe('SearchPage', () => {
  let timer: sinon.SinonFakeTimers;
  beforeEach(() => {
    timer = sinon.useFakeTimers();
  });
  afterEach(() => {
    timer.restore();
    const url = `${window.location.protocol}//${window.location.host}${window.location.pathname}`;
    window.history.replaceState(null, '', url);
  });

  it('should throttle search query updates', () => {
    const store = Store.create({});
    after(() => destroy(store));
    unprotect(store);
    const setSearchQuerySpy = sinon.spy(store.searchPage, 'setSearchQuery');
    protect(store);

    render(
      <StoreProvider value={store}>
        <SearchPage />
      </StoreProvider>
    );

    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'builder' } });
    timer.tick(10);
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'builder-id' } });
    timer.tick(10);
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'builder-id-with-suffix' } });
    timer.tick(300);
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'another-builder' } });
    timer.tick(10);
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'another-builder-id' } });
    timer.runAll();

    expect(setSearchQuerySpy.callCount).to.eq(2);
    expect(setSearchQuerySpy.getCall(0).args).to.deep.eq(['builder-id-with-suffix']);
    expect(setSearchQuerySpy.getCall(1).args).to.deep.eq(['another-builder-id']);
  });

  it('should cancel search query updates when switching search targets', () => {
    const store = Store.create({});
    after(() => destroy(store));
    unprotect(store);
    const setSearchQuerySpy = sinon.spy(store.searchPage, 'setSearchQuery');
    protect(store);

    render(
      <StoreProvider value={store}>
        <SearchPage />
      </StoreProvider>
    );

    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'builder' } });
    timer.tick(10);
    store.searchPage.setSearchTarget(SearchTarget.Tests);
    timer.tick(10);
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'test-id' } });
    timer.runAll();

    expect(setSearchQuerySpy.callCount).to.eq(1);
    expect(setSearchQuerySpy.getCall(0).args).to.deep.eq(['test-id']);
  });

  it('should read params from URL', async () => {
    const url = new URLExt(window.location.href)
      .setSearchParam('t', SearchTarget.Tests)
      .setSearchParam('tp', 'project')
      .setSearchParam('q', 'query');
    window.history.replaceState(null, '', url);

    const store = Store.create({});
    after(() => destroy(store));

    render(
      <StoreProvider value={store}>
        <SearchPage />
      </StoreProvider>
    );

    expect(store.searchPage.searchTarget).to.eq(SearchTarget.Tests);
    expect(store.searchPage.testProject).to.eq('project');
    expect(store.searchPage.searchQuery).to.eq('query');

    // Ensure that the filters are not overwritten after all event hooks are
    // executed.
    await timer.runAllAsync();

    expect(store.searchPage.searchTarget).to.eq(SearchTarget.Tests);
    expect(store.searchPage.testProject).to.eq('project');
    expect(store.searchPage.searchQuery).to.eq('query');
  });

  it('should sync params with URL', async () => {
    const store = Store.create({});
    after(() => destroy(store));

    render(
      <StoreProvider value={store}>
        <SearchPage />
      </StoreProvider>
    );

    expect(window.location.search).to.eq('');
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'query' } });
    store.searchPage.setSearchTarget(SearchTarget.Tests);
    store.searchPage.setTestProject('project');

    await timer.runAllAsync();

    expect(window.location.search).to.eq('?t=TESTS&tp=project&q=query');
  });
});
