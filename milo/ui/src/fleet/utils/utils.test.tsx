// Copyright 2025 The LUCI Authors.
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

import { render, fireEvent } from '@testing-library/react';
import React from 'react';

import { keyboardListNavigationHandler } from './utils';

const TestMenu = ({ deepNestLi = false }: { deepNestLi?: boolean }) => (
  <ul
    role="menu"
    onKeyDown={(e) =>
      keyboardListNavigationHandler(e as React.KeyboardEvent<HTMLUListElement>)
    }
  >
    <div id="search" data-testid="search-container" tabIndex={-1}>
      <input data-testid="search-input" />
    </div>
    {deepNestLi ? (
      <div>
        <div>
          <li role="menuitem" tabIndex={0} data-testid="item-1">
            Item 1
          </li>
        </div>
      </div>
    ) : (
      <li role="menuitem" tabIndex={0} data-testid="item-1">
        Item 1
      </li>
    )}
    <li role="menuitem" tabIndex={0} data-testid="item-2">
      Item 2
    </li>
    <div>
      <button tabIndex={-1} data-testid="footer-button">
        Reset
      </button>
    </div>
  </ul>
);

describe('keyboardListNavigationHandler with React Testing Library', () => {
  it('should move focus from search to the first item on ArrowDown', () => {
    const { getByTestId } = render(<TestMenu />);
    const searchInput = getByTestId('search-input');
    const item1 = getByTestId('item-1');

    searchInput.focus();
    fireEvent.keyDown(searchInput, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(item1);
  });

  it('should move focus from the last item to the footer button on ArrowDown', () => {
    const { getByTestId } = render(<TestMenu />);
    const item2 = getByTestId('item-2');
    const footerButton = getByTestId('footer-button');

    item2.focus();
    fireEvent.keyDown(item2, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(footerButton);
  });

  it('should wrap focus from the footer button to the search container on ArrowDown', () => {
    const { getByTestId } = render(<TestMenu />);
    const footerButton = getByTestId('footer-button');
    const searchContainer = getByTestId('search-container');

    footerButton.focus();
    fireEvent.keyDown(footerButton, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(searchContainer);
  });

  it('should move focus from the first item to the search container on ArrowUp', () => {
    const { getByTestId } = render(<TestMenu />);
    const item1 = getByTestId('item-1');
    const searchContainer = getByTestId('search-container');

    item1.focus();
    fireEvent.keyDown(item1, { key: 'ArrowUp' });
    expect(document.activeElement).toBe(searchContainer);
  });

  it('should move focus from the search container to the footer button on ArrowUp', () => {
    const { getByTestId } = render(<TestMenu />);
    const searchContainer = getByTestId('search-container');
    const footerButton = getByTestId('footer-button');

    searchContainer.focus();
    fireEvent.keyDown(searchContainer, { key: 'ArrowUp' });
    expect(document.activeElement).toBe(footerButton);
  });

  it('should correctly navigate when a menuitem is deeply nested', () => {
    const { getByTestId } = render(<TestMenu deepNestLi />);
    const searchInput = getByTestId('search-input');
    const item1 = getByTestId('item-1');
    const item2 = getByTestId('item-2');

    searchInput.focus();
    fireEvent.keyDown(searchInput, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(item1);

    item1.focus();
    fireEvent.keyDown(item1, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(item2);
  });
});
