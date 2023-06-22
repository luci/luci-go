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

import { afterEach, beforeEach, expect, jest } from '@jest/globals';
import { cleanup, render } from '@testing-library/react';
import { applySnapshot, destroy, Instance } from 'mobx-state-tree';
import type { NavigateFunction } from 'react-router-dom';
import * as reactRouterDom from 'react-router-dom';

import { Store, StoreProvider } from '@/common/store';

import { BuildDefaultTab } from './build_default_tab';

jest.mock('react-router-dom', () => {
  const actualReactRouterDom = jest.requireActual(
    'react-router-dom'
  ) as typeof reactRouterDom;
  const mockedReactRouterDom = {
    ...actualReactRouterDom,
    // Wraps `useNavigate` in a mock so we can mock its implementation later.
    useNavigate: jest.fn(actualReactRouterDom.useNavigate),
  };
  return mockedReactRouterDom;
});

describe('BuildDefaultTab', () => {
  let store: Instance<typeof Store>;
  let useNavigateSpy: jest.Mock<() => jest.Mock<NavigateFunction>>;

  beforeEach(() => {
    jest.useFakeTimers();
    useNavigateSpy = reactRouterDom.useNavigate as unknown as jest.Mock<
      () => jest.Mock<NavigateFunction>
    >;
    const navigateSpies = new Map<
      NavigateFunction,
      jest.Mock<NavigateFunction>
    >();
    useNavigateSpy.mockImplementation(() => {
      const navigate = (
        jest.requireActual('react-router-dom') as typeof reactRouterDom
      ).useNavigate();
      // Return the same mock reference if the reference to `navigate` is the
      // same. This is to ensure the dependency checks having the same result.
      const navigateSpy = navigateSpies.get(navigate) || jest.fn(navigate);
      navigateSpies.set(navigate, navigateSpy);
      return navigateSpy;
    });
    store = Store.create({ userConfig: { build: { defaultTab: 'overview' } } });
  });

  afterEach(() => {
    cleanup();
    destroy(store);
    jest.useRealTimers();
    useNavigateSpy.mockRestore();
  });

  test('should redirect to the default tab', async () => {
    const router = reactRouterDom.createMemoryRouter(
      [
        {
          path: 'path/prefix',
          children: [
            { index: true, element: <BuildDefaultTab /> },
            { path: 'overview', element: <></> },
          ],
        },
      ],
      { initialEntries: ['/path/prefix?param#hash'] }
    );

    render(
      <StoreProvider value={store}>
        <reactRouterDom.RouterProvider router={router} />
      </StoreProvider>
    );

    expect(useNavigateSpy).toHaveBeenCalledTimes(1);
    const useNavigateSpyResult = useNavigateSpy.mock.results[0];
    expect(useNavigateSpyResult.type).toEqual('return');
    // Won't happen. Useful for type inference.
    if (useNavigateSpyResult.type !== 'return') {
      throw new Error('unreachable');
    }
    const navigateSpy = useNavigateSpyResult.value;
    expect(navigateSpy).toHaveBeenCalledTimes(1);
    expect(navigateSpy.mock.calls[0]).toMatchObject([
      // The type definition for `.toMatchObject` is incomplete. Cast to any to
      // make TSC happy.
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      '/path/prefix/overview?param#hash' as any,
      { replace: true },
    ]);
  });

  test("should work with '/' suffix", async () => {
    const router = reactRouterDom.createMemoryRouter(
      [
        {
          path: 'path/prefix',
          children: [
            { index: true, element: <BuildDefaultTab /> },
            { path: 'overview', element: <></> },
          ],
        },
      ],
      { initialEntries: ['/path/prefix/?param#hash'] }
    );

    render(
      <StoreProvider value={store}>
        <reactRouterDom.RouterProvider router={router} />
      </StoreProvider>
    );

    expect(useNavigateSpy).toHaveBeenCalledTimes(1);
    const useNavigateSpyResult = useNavigateSpy.mock.results[0];
    expect(useNavigateSpyResult.type).toEqual('return');
    // Won't happen. Useful for type inference.
    if (useNavigateSpyResult.type !== 'return') {
      throw new Error('unreachable');
    }
    const navigateSpy = useNavigateSpyResult.value;
    expect(navigateSpy).toHaveBeenCalledTimes(1);
    expect(navigateSpy.mock.calls[0]).toMatchObject([
      // The type definition for `.toMatchObject` is incomplete. Cast to any to
      // make TSC happy.
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      '/path/prefix/overview?param#hash' as any,
      { replace: true },
    ]);
  });

  test('should not cause infinite redirects', async () => {
    // Set the default tab to redirect to itself.
    // This won't happen in prod but useful for testing purpose.
    applySnapshot(store, { userConfig: { build: { defaultTab: '' } } });

    const router = reactRouterDom.createMemoryRouter(
      [
        {
          path: 'path/prefix',
          children: [{ index: true, element: <BuildDefaultTab /> }],
        },
      ],
      { initialEntries: ['/path/prefix/?param#hash'] }
    );

    render(
      <StoreProvider value={store}>
        <reactRouterDom.RouterProvider router={router} />
      </StoreProvider>
    );

    expect(useNavigateSpy).toHaveBeenCalledTimes(2);
    // Ensures the same `navigate` function is returned in both calls.
    // This should always pass unless `react-router-dom` changes the
    // implementation of `useNavigate`.
    expect(useNavigateSpy.mock.results[0].value).toBe(
      useNavigateSpy.mock.results[1].value
    );

    const useNavigateSpyResult = useNavigateSpy.mock.results[0];
    expect(useNavigateSpyResult.type).toEqual('return');
    // Won't happen. Useful for type inference.
    if (useNavigateSpyResult.type !== 'return') {
      throw new Error('unreachable');
    }
    const navigateSpy = useNavigateSpyResult.value;
    // Still only called once.
    expect(navigateSpy).toHaveBeenCalledTimes(1);
    expect(navigateSpy.mock.calls[0]).toMatchObject([
      // The type definition for `.toMatchObject` is incomplete. Cast to any to
      // make TSC happy.
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      '/path/prefix/?param#hash' as any,
      { replace: true },
    ]);
  });
});
