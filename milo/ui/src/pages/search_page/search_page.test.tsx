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

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from '@testing-library/react';
import { destroy, protect, unprotect } from 'mobx-state-tree';

import { Store, StoreInstance, StoreProvider } from '@/common/store';
import { SearchTarget } from '@/common/store/search_page';
import { URLExt } from '@/generic_libs/tools/utils';

import { SearchPage } from './search_page';

jest.mock('./builder_list', () => ({
  BuilderList: () => <></>,
}));

describe('SearchPage', () => {
  let store: StoreInstance;
  beforeEach(() => {
    jest.useFakeTimers();
    store = Store.create({});
    const url = `${window.location.protocol}//${window.location.host}${window.location.pathname}`;
    window.history.replaceState(null, '', url);
  });
  afterEach(() => {
    cleanup();
    destroy(store);
    jest.useRealTimers();
  });

  test('should throttle search query updates', () => {
    unprotect(store);
    const setSearchQuerySpy = jest.spyOn(store.searchPage, 'setSearchQuery');
    protect(store);

    render(
      <StoreProvider value={store}>
        <SearchPage />
      </StoreProvider>
    );

    fireEvent.change(screen.getByTestId('filter-input'), {
      target: { value: 'builder' },
    });

    act(() => jest.advanceTimersByTime(10));
    fireEvent.change(screen.getByTestId('filter-input'), {
      target: { value: 'builder-id' },
    });
    act(() => jest.advanceTimersByTime(10));
    fireEvent.change(screen.getByTestId('filter-input'), {
      target: { value: 'builder-id-with-suffix' },
    });
    act(() => jest.advanceTimersByTime(300));
    fireEvent.change(screen.getByTestId('filter-input'), {
      target: { value: 'another-builder' },
    });
    act(() => jest.advanceTimersByTime(10));
    fireEvent.change(screen.getByTestId('filter-input'), {
      target: { value: 'another-builder-id' },
    });
    act(() => jest.runAllTimers());

    expect(setSearchQuerySpy.mock.calls.length).toStrictEqual(2);
    expect(setSearchQuerySpy.mock.calls[0]).toEqual(['builder-id-with-suffix']);
    expect(setSearchQuerySpy.mock.calls[1]).toEqual(['another-builder-id']);
  });

  test('should cancel search query updates when switching search targets', async () => {
    unprotect(store);
    const setSearchQuerySpy = jest.spyOn(store.searchPage, 'setSearchQuery');
    protect(store);

    render(
      <StoreProvider value={store}>
        <SearchPage />
      </StoreProvider>
    );

    fireEvent.change(screen.getByTestId('filter-input'), {
      target: { value: 'builder' },
    });
    act(() => jest.advanceTimersByTime(10));
    act(() => store.searchPage.setSearchTarget(SearchTarget.Tests));
    act(() => jest.advanceTimersByTime(10));
    fireEvent.change(screen.getByTestId('filter-input'), {
      target: { value: 'test-id' },
    });
    act(() => jest.runAllTimers());

    expect(setSearchQuerySpy.mock.calls.length).toStrictEqual(1);
    expect(setSearchQuerySpy.mock.calls[0]).toEqual(['test-id']);
  });

  test('should read params from URL', async () => {
    const url = new URLExt(window.location.href)
      .setSearchParam('t', SearchTarget.Tests)
      .setSearchParam('tp', 'project')
      .setSearchParam('q', 'query');
    window.history.replaceState(null, '', url);

    render(
      <StoreProvider value={store}>
        <SearchPage />
      </StoreProvider>
    );

    expect(store.searchPage.searchTarget).toStrictEqual(SearchTarget.Tests);
    expect(store.searchPage.testProject).toStrictEqual('project');
    expect(store.searchPage.searchQuery).toStrictEqual('query');

    // Ensure that the filters are not overwritten after all event hooks are
    // executed.
    await act(async () => await jest.runAllTimersAsync());

    expect(store.searchPage.searchTarget).toStrictEqual(SearchTarget.Tests);
    expect(store.searchPage.testProject).toStrictEqual('project');
    expect(store.searchPage.searchQuery).toStrictEqual('query');
  });

  test('should sync params with URL', async () => {
    render(
      <StoreProvider value={store}>
        <SearchPage />
      </StoreProvider>
    );

    expect(window.location.search).toStrictEqual('');
    fireEvent.change(screen.getByTestId('filter-input'), {
      target: { value: 'query' },
    });

    act(() => {
      store.searchPage.setSearchTarget(SearchTarget.Tests);
      store.searchPage.setTestProject('project');
    });

    await act(async () => await jest.runAllTimersAsync());

    expect(window.location.search).toStrictEqual('?t=TESTS&tp=project&q=query');
  });
});
