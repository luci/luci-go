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

import { Store, StoreProvider } from '../../store';
import { SearchTarget } from '../../store/search_page';
import { SearchPage } from '.';

describe('SearchPage', () => {
  let timer: sinon.SinonFakeTimers;
  beforeEach(() => {
    timer = sinon.useFakeTimers();
  });
  afterEach(() => timer.restore());

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
});
