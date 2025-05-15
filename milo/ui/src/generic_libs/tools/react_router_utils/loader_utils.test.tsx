// Copyright 2024 The LUCI Authors.
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
import { act } from 'react';
import { RouterProvider, createMemoryRouter } from 'react-router';

import { URLObserver } from '@/testing_tools/url_observer';

import { redirectToChildPath } from './loader_utils';

describe('redirectToChildPath', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    jest.useRealTimers();
  });

  it('should redirect to the default tab', async () => {
    const urlCallback = jest.fn();
    const router = createMemoryRouter(
      [
        {
          path: 'path/prefix',
          children: [
            { index: true, loader: redirectToChildPath('my-default-tab') },
            {
              path: 'my-default-tab',
              element: <URLObserver callback={urlCallback} />,
            },
          ],
        },
      ],
      { initialEntries: ['/path/prefix?param#hash'] },
    );

    render(<RouterProvider router={router} />);
    await act(() => jest.runOnlyPendingTimersAsync());

    expect(urlCallback).toHaveBeenCalledTimes(1);
    expect(urlCallback).toHaveBeenLastCalledWith(
      expect.objectContaining({
        pathname: '/path/prefix/my-default-tab',
        search: '?param',
      }),
    );
  });

  it("should work with '/' suffix", async () => {
    const urlCallback = jest.fn();
    const router = createMemoryRouter(
      [
        {
          path: 'path/prefix',
          children: [
            { index: true, loader: redirectToChildPath('my-default-tab') },
            {
              path: 'my-default-tab',
              element: <URLObserver callback={urlCallback} />,
            },
          ],
        },
      ],
      { initialEntries: ['/path/prefix/?param#hash'] },
    );

    render(<RouterProvider router={router} />);
    await act(() => jest.runOnlyPendingTimersAsync());

    expect(urlCallback).toHaveBeenCalledTimes(1);
    expect(urlCallback).toHaveBeenLastCalledWith(
      expect.objectContaining({
        pathname: '/path/prefix/my-default-tab',
        search: '?param',
      }),
    );
  });
});
