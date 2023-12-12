// Copyright 2023 The LUCI Authors.
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

import { render, screen } from '@testing-library/react';
import { destroy } from 'mobx-state-tree';

import { UiPage } from '@/common/constants/view';
import { Store, StoreInstance, StoreProvider } from '@/common/store';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { SIDE_BAR_OPEN_CACHE_KEY } from './base_layout';
import { BaseLayout } from './base_layout';
import { PAGE_LABEL_MAP } from './constants';

describe('BaseLayout', () => {
  const storageList = new Map<string, string>();
  let store: StoreInstance;

  beforeEach(() => {
    jest
      .spyOn(Storage.prototype, 'setItem')
      .mockImplementation((key: string, val: string) => {
        storageList.set(key, val);
      });
    jest
      .spyOn(Storage.prototype, 'getItem')
      .mockImplementation((key: string) => {
        return storageList.get(key) ?? null;
      });
    store = Store.create({});
  });

  afterEach(() => {
    jest.restoreAllMocks();
    storageList.clear();
    destroy(store);
  });

  it('should display sidebar if no value is stored', async () => {
    render(
      <StoreProvider value={store}>
        <FakeContextProvider>
          <BaseLayout />
        </FakeContextProvider>
      </StoreProvider>,
    );

    await screen.findByText('LUCI');

    expect(
      screen.getByText(PAGE_LABEL_MAP[UiPage.BuilderSearch]),
    ).toBeVisible();
  });

  it('should hide sidebar if the local storage value is false', async () => {
    storageList.set(SIDE_BAR_OPEN_CACHE_KEY, 'false');
    render(
      <StoreProvider value={store}>
        <FakeContextProvider>
          <BaseLayout />
        </FakeContextProvider>
      </StoreProvider>,
    );

    await screen.findByText('LUCI');

    expect(
      screen.getByText(PAGE_LABEL_MAP[UiPage.BuilderSearch]),
    ).not.toBeVisible();
  });
});
