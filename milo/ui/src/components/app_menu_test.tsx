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

import { Store, StoreProvider } from '../store';
import { AppMenu } from './app_menu';

describe('AppMenu', () => {
  it('has pending update', () => {
    const store = Store.create({});
    after(() => destroy(store));
    unprotect(store);
    sinon.stub(store.workbox, 'hasPendingUpdate').get(() => true);
    protect(store);

    render(
      <StoreProvider value={store}>
        <AppMenu />
      </StoreProvider>
    );

    const menuButton = screen.getByTestId('menu-button');
    expect(menuButton.querySelector('[data-testid="UpgradeIcon"]')).to.not.be.null;

    fireEvent.click(menuButton);
    const menu = screen.getByRole('menuitem', { name: 'update website' });
    expect(menu.ariaDisabled).to.not.eq('true');
  });

  it('no pending update', () => {
    const store = Store.create({});
    after(() => destroy(store));
    unprotect(store);
    sinon.stub(store.workbox, 'hasPendingUpdate').get(() => false);
    protect(store);

    render(
      <StoreProvider value={store}>
        <AppMenu />
      </StoreProvider>
    );

    const menuButton = screen.getByTestId('menu-button');
    expect(menuButton.querySelector('[data-testid="UpgradeIcon"]')).to.be.null;

    fireEvent.click(menuButton);
    const menu = screen.getByRole('menuitem', { name: 'update website' });
    expect(menu.ariaDisabled).to.eq('true');
  });
});
