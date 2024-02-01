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

import { cleanup, render } from '@testing-library/react';
import type { NavigateFunction } from 'react-router-dom';
import * as reactRouterDom from 'react-router-dom';

import { InvocationDefaultTab } from './invocation_default_tab';

jest.mock('react-router-dom', () => {
  return createSelectiveMockFromModule<typeof import('react-router-dom')>(
    'react-router-dom',
    ['useNavigate'],
  );
});

describe('InvocationDefaultTab', () => {
  let useNavigateSpy: jest.MockedFunction<
    () => jest.MockedFunction<NavigateFunction>
  >;

  beforeEach(() => {
    jest.useFakeTimers();
    const navigateSpies = new Map<
      NavigateFunction,
      jest.MockedFunction<NavigateFunction>
    >();
    useNavigateSpy = jest
      .mocked(
        // We will return a mocked `navigate` function so we can intercept the
        // `navigate` calls.
        reactRouterDom.useNavigate as () => jest.MockedFunction<NavigateFunction>,
      )
      .mockImplementation(() => {
        const navigate = (
          jest.requireActual('react-router-dom') as typeof reactRouterDom
        ).useNavigate();
        // Return the same mock reference if the reference to `navigate` is the
        // same. This is to ensure the dependency checks having the same result.
        const navigateSpy =
          navigateSpies.get(navigate) ||
          (jest.fn(
            navigate,
            // `jest.fn` isn't smart enough to infer the function type when
            // mocking an overloaded function. Use manual casting instead.
          ) as unknown as jest.MockedFunction<NavigateFunction>);
        navigateSpies.set(navigate, navigateSpy);
        return navigateSpy;
      });
  });

  afterEach(() => {
    cleanup();
    jest.useRealTimers();
    useNavigateSpy.mockRestore();
  });

  test('should redirect to the default tab', async () => {
    const router = reactRouterDom.createMemoryRouter(
      [
        {
          path: 'path/prefix',
          children: [
            { index: true, element: <InvocationDefaultTab /> },
            { path: 'test-results', element: <></> },
          ],
        },
      ],
      { initialEntries: ['/path/prefix?param#hash'] },
    );

    render(<reactRouterDom.RouterProvider router={router} />);

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
      '/path/prefix/test-results?param#hash' as any,
      { replace: true },
    ]);
  });

  test("should work with '/' suffix", async () => {
    const router = reactRouterDom.createMemoryRouter(
      [
        {
          path: 'path/prefix',
          children: [
            { index: true, element: <InvocationDefaultTab /> },
            { path: 'test-results', element: <></> },
          ],
        },
      ],
      { initialEntries: ['/path/prefix/?param#hash'] },
    );

    render(<reactRouterDom.RouterProvider router={router} />);

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
      '/path/prefix/test-results?param#hash' as any,
      { replace: true },
    ]);
  });
});
