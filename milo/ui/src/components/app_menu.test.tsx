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

import { afterEach, beforeEach, expect, jest } from '@jest/globals';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { destroy, protect, unprotect } from 'mobx-state-tree';

import { Store, StoreInstance, StoreProvider } from '../store';
import { AppMenu } from './app_menu';

describe('AppMenu', () => {
  let store: StoreInstance;
  beforeEach(() => {
    store = Store.create({});
  });
  afterEach(() => {
    cleanup();
    unprotect(store);
    jest.restoreAllMocks();
    protect(store);
    destroy(store);
  });

  it('has pending update', () => {
    unprotect(store);
    jest
      .spyOn(store.workbox, 'hasPendingUpdate', 'get')
      .mockImplementation(() => true);
    protect(store);

    render(
      <StoreProvider value={store}>
        <AppMenu />
      </StoreProvider>
    );

    const menuButton = screen.getByTestId('menu-button');
    expect(
      menuButton.querySelector('[data-testid="UpgradeIcon"]')
    ).not.toBeNull();

    fireEvent.click(menuButton);
    const menu = screen.getByRole('menuitem', { name: 'update website' });
    expect(menu.getAttribute('aria-disabled')).not.toStrictEqual('true');
  });

  it('no pending update', () => {
    unprotect(store);
    jest
      .spyOn(store.workbox, 'hasPendingUpdate', 'get')
      .mockImplementation(() => false);
    protect(store);

    expect(store.workbox.hasPendingUpdate).toStrictEqual(false);

    render(
      <StoreProvider value={store}>
        <AppMenu />
      </StoreProvider>
    );

    const menuButton = screen.getByTestId('menu-button');
    expect(menuButton.querySelector('[data-testid="UpgradeIcon"]')).toBeNull();

    fireEvent.click(menuButton);
    const menu = screen.getByRole('menuitem', { name: 'update website' });
    expect(menu.getAttribute('aria-disabled')).toStrictEqual('true');
  });
});
