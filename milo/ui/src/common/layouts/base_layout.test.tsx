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
import { useMatches } from 'react-router';

import * as PageMeta from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { Store, StoreInstance, StoreProvider } from '@/common/store';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { SIDE_BAR_OPEN_CACHE_KEY } from './base_layout';
import { BaseLayout } from './base_layout';
import { PAGE_LABEL_MAP } from './constants';

jest.mock('@/common/tools/logging', () => ({
  logging: {
    warn: jest.fn(),
    error: jest.fn(),
  },
}));

jest.mock('@/common/components/page_meta', () => ({
  ...jest.requireActual('@/common/components/page_meta'),
  useProjectCtx: jest.fn(),
}));

jest.mock('react-router', () => {
  return self.createSelectiveSpiesFromModule<typeof import('react-router')>(
    'react-router',
    ['useMatches'],
  );
});

describe('<BaseLayout />', () => {
  const storageList = new Map<string, string>();
  let store: StoreInstance;
  let useMatchesMock: jest.MockedFunctionDeep<typeof useMatches>;

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

    useMatchesMock = jest.mocked(useMatches);
  });

  afterEach(() => {
    jest.restoreAllMocks();
    storageList.clear();
    destroy(store);

    useMatchesMock.mockClear();
  });

  it('should not display sidebar if no value is stored', async () => {
    // Default is now closed (false)
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

  it('should hide sidebar if project is android even if local storage is true', async () => {
    storageList.set(SIDE_BAR_OPEN_CACHE_KEY, 'true');
    (PageMeta.useProjectCtx as jest.Mock).mockReturnValue('android');

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

  it('should respect custom layouts', async () => {
    useMatchesMock.mockImplementation(() => [
      {
        id: '',
        pathname: '',
        params: {},
        data: '',
        handle: {
          layout: () => <div data-testid="test-layout"> Custom Layout </div>,
        },
        loaderData: [],
      },
    ]);

    render(
      <StoreProvider value={store}>
        <FakeContextProvider>
          <BaseLayout />
        </FakeContextProvider>
      </StoreProvider>,
    );

    expect(screen.queryByTestId('test-layout')).toBeInTheDocument();
  });

  it('should pick the deepest custom layouts', async () => {
    useMatchesMock.mockImplementation(() => [
      {
        id: '',
        pathname: '',
        params: {},
        data: '',
        handle: {
          layout: () => <div data-testid="layoutl1"> Custom Layout </div>,
        },
        loaderData: [],
      },
      {
        id: '',
        pathname: '',
        params: {},
        data: '',
        handle: {
          layout: () => <div data-testid="layoutl2"> Custom Layout </div>,
        },
        loaderData: [],
      },
      {
        id: '',
        pathname: '',
        params: {},
        data: '',
        handle: {
          layout: () => <div data-testid="layoutl3"> Custom Layout </div>,
        },
        loaderData: [],
      },
    ]);

    render(
      <StoreProvider value={store}>
        <FakeContextProvider>
          <BaseLayout />
        </FakeContextProvider>
      </StoreProvider>,
    );

    expect(screen.queryByTestId('layoutl1')).not.toBeInTheDocument();
    expect(screen.queryByTestId('layoutl2')).not.toBeInTheDocument();
    expect(screen.queryByTestId('layoutl3')).toBeInTheDocument();
  });

  it('should use the default layout if no customs are specified', async () => {
    useMatchesMock.mockImplementation(() => []);

    render(
      <StoreProvider value={store}>
        <FakeContextProvider>
          <BaseLayout />
        </FakeContextProvider>
      </StoreProvider>,
    );

    expect(screen.queryByTitle('Send feedback')).toBeInTheDocument();
    expect(screen.queryByText('Privacy')).toBeInTheDocument();
  });
});
