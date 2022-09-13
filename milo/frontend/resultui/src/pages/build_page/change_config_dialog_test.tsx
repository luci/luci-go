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
import { destroy, Instance, protect, unprotect } from 'mobx-state-tree';
import * as sinon from 'sinon';

import { Store, StoreProvider } from '../../store';
import { ChangeConfigDialog } from './change_config_dialog';

describe('ChangeConfigDialog', () => {
  let timer: sinon.SinonFakeTimers;
  let store: Instance<typeof Store>;
  let setDefaultTabSpy: sinon.SinonSpy<[string], void>;
  beforeEach(() => {
    timer = sinon.useFakeTimers();
    store = Store.create();
    unprotect(store);
    setDefaultTabSpy = sinon.spy(store.userConfig.build, 'setDefaultTab');
    protect(store);
  });

  afterEach(() => {
    timer.restore();
    destroy(store);
  });

  it('should sync local state when opening the dialog', async () => {
    store.userConfig.build.setDefaultTab('build-test-results');
    const { rerender } = render(
      <StoreProvider value={store}>
        <ChangeConfigDialog open />
      </StoreProvider>
    );

    expect(screen.queryByRole('button', { name: 'Test Results' })).to.not.be.null;
    expect(screen.queryByRole('button', { name: 'Timeline' })).to.be.null;

    store.userConfig.build.setDefaultTab('build-timeline');
    await timer.runToLastAsync();

    // Updating the config while the dialog is still open has no effect.
    expect(screen.queryByRole('button', { name: 'Test Results' })).to.not.be.null;
    expect(screen.queryByRole('button', { name: 'Timeline' })).to.be.null;

    rerender(
      <StoreProvider value={store}>
        <ChangeConfigDialog open={false} />
      </StoreProvider>
    );
    rerender(
      <StoreProvider value={store}>
        <ChangeConfigDialog open />
      </StoreProvider>
    );

    expect(screen.queryByRole('button', { name: 'Test Results' })).to.be.null;
    expect(screen.queryByRole('button', { name: 'Timeline' })).to.not.be.null;
  });

  it('should update global config when confirmed', async () => {
    store.userConfig.build.setDefaultTab('build-test-results');
    const onCloseSpy = sinon.spy();

    render(
      <StoreProvider value={store}>
        <ChangeConfigDialog open onClose={onCloseSpy} />
      </StoreProvider>
    );

    expect(screen.queryByRole('button', { name: 'Test Results' })).to.not.be.null;
    expect(screen.queryByRole('button', { name: 'Timeline' })).to.be.null;

    fireEvent.mouseDown(screen.getByRole('button', { name: 'Test Results' }));
    await timer.runToLastAsync();
    fireEvent.click(screen.getByText('Timeline'));
    await timer.runToLastAsync();

    expect(screen.queryByRole('button', { name: 'Test Results' })).to.be.null;
    expect(screen.queryByRole('button', { name: 'Timeline' })).to.not.be.null;

    expect(onCloseSpy.callCount).to.eq(0);
    expect(setDefaultTabSpy.callCount).to.eq(1);

    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await timer.runToLastAsync();

    expect(onCloseSpy.callCount).to.eq(1);
    expect(setDefaultTabSpy.callCount).to.eq(2);
    expect(store.userConfig.build.defaultTabName).to.eq('build-timeline');
  });

  it('should not update global config when dismissed', async () => {
    store.userConfig.build.setDefaultTab('build-test-results');
    const onCloseSpy = sinon.spy();

    render(
      <StoreProvider value={store}>
        <ChangeConfigDialog open onClose={onCloseSpy} />
      </StoreProvider>
    );

    expect(screen.queryByRole('button', { name: 'Test Results' })).to.not.be.null;
    expect(screen.queryByRole('button', { name: 'Timeline' })).to.be.null;

    fireEvent.mouseDown(screen.getByRole('button', { name: 'Test Results' }));
    await timer.runToLastAsync();
    fireEvent.click(screen.getByText('Timeline'));
    await timer.runToLastAsync();

    expect(screen.queryByRole('button', { name: 'Test Results' })).to.be.null;
    expect(screen.queryByRole('button', { name: 'Timeline' })).to.not.be.null;

    expect(onCloseSpy.callCount).to.eq(0);
    expect(setDefaultTabSpy.callCount).to.eq(1);

    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
    await timer.runToLastAsync();

    expect(onCloseSpy.callCount).to.eq(1);
    expect(setDefaultTabSpy.callCount).to.eq(1);
    expect(store.userConfig.build.defaultTabName).to.eq('build-test-results');
  });
});
