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

import { cleanup, render, screen } from '@testing-library/react';
import { act } from 'react';
import { Link, RouterProvider, createMemoryRouter } from 'react-router-dom';

import { useDeclareTabId } from './context';
import { RoutedTab } from './routed_tab';
import { RoutedTabs } from './routed_tabs';

function TabA() {
  useDeclareTabId('tab-a');
  return <>tab A content</>;
}

function TabB() {
  useDeclareTabId('tab-b');
  return <>tab B content</>;
}

describe('RoutedTabs', () => {
  afterEach(() => {
    cleanup();
  });

  it('should mark the active tab correctly', () => {
    const router = createMemoryRouter(
      [
        {
          path: 'path/prefix',
          element: (
            <RoutedTabs>
              <RoutedTab label="tab A label" value="tab-a" to="tab-a" />
              <RoutedTab label="tab B label" value="tab-b" to="tab-b" />
            </RoutedTabs>
          ),
          children: [
            { path: 'tab-a', element: <TabA /> },
            { path: 'tab-b', element: <TabB /> },
          ],
        },
      ],
      { initialEntries: ['/path/prefix/tab-a'] },
    );
    render(<RouterProvider router={router} />);

    expect(screen.getByText('tab A label')).toHaveAttribute(
      'aria-selected',
      'true',
    );
    expect(screen.queryByText('tab A content')).toBeInTheDocument();

    expect(screen.getByText('tab B label')).not.toHaveAttribute(
      'aria-selected',
      'true',
    );
    expect(screen.queryByText('tab B content')).not.toBeInTheDocument();

    act(() => screen.getByText('tab B label').click());

    expect(screen.getByText('tab A label')).not.toHaveAttribute(
      'aria-selected',
      'true',
    );
    expect(screen.queryByText('tab A content')).not.toBeInTheDocument();

    expect(screen.getByText('tab B label')).toHaveAttribute(
      'aria-selected',
      'true',
    );
    expect(screen.queryByText('tab B content')).toBeInTheDocument();
  });

  it('hideWhenInactive should work correctly', () => {
    const router = createMemoryRouter(
      [
        {
          path: 'path/prefix',
          element: (
            <>
              <Link to="tab-b">direct link to tab B</Link>
              <RoutedTabs>
                <RoutedTab label="tab A label" value="tab-a" to="tab-a" />
                <RoutedTab
                  label="tab B label"
                  value="tab-b"
                  to="tab-b"
                  hideWhenInactive
                />
              </RoutedTabs>
            </>
          ),
          children: [
            { path: 'tab-a', element: <TabA /> },
            { path: 'tab-b', element: <TabB /> },
          ],
        },
      ],
      { initialEntries: ['/path/prefix/tab-a'] },
    );
    render(<RouterProvider router={router} />);

    expect(screen.queryByText('tab B label')).not.toBeInTheDocument();

    act(() => screen.getByText('direct link to tab B').click());
    expect(screen.getByText('tab B label')).not.toHaveStyleRule(
      'display',
      'none',
    );
  });
});
