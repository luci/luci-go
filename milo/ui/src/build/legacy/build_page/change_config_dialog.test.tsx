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

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from '@testing-library/react';
import { destroy, Instance, protect, unprotect } from 'mobx-state-tree';

import { Store, StoreProvider } from '@/common/store';

import { ChangeConfigDialog } from './change_config_dialog';

describe('ChangeConfigDialog', () => {
  let store: Instance<typeof Store>;
  let setDefaultTabSpy: jest.SpiedFunction<(tab: string) => void>;
  beforeEach(() => {
    jest.useFakeTimers();
    store = Store.create();
    unprotect(store);
    setDefaultTabSpy = jest.spyOn(store.userConfig.build, 'setDefaultTab');
    protect(store);
  });

  afterEach(() => {
    cleanup();
    destroy(store);
    jest.useRealTimers();
  });

  it('should sync local state when opening the dialog', async () => {
    store.userConfig.build.setDefaultTab('test-results');
    const { rerender } = render(
      <StoreProvider value={store}>
        <ChangeConfigDialog open />
      </StoreProvider>,
    );

    const tabSwitch = screen.getByRole('combobox');

    expect(tabSwitch).toHaveTextContent('Test Results');

    act(() => store.userConfig.build.setDefaultTab('timeline'));
    await act(() => jest.runOnlyPendingTimersAsync());

    // Updating the config while the dialog is still open has no effect.
    expect(tabSwitch).toHaveTextContent('Test Results');

    rerender(
      <StoreProvider value={store}>
        <ChangeConfigDialog open={false} />
      </StoreProvider>,
    );

    expect(tabSwitch).toHaveTextContent('Test Results');
  });

  it('should update global config when confirmed', async () => {
    store.userConfig.build.setDefaultTab('test-results');
    const onCloseSpy = jest.fn();

    render(
      <StoreProvider value={store}>
        <ChangeConfigDialog open onClose={onCloseSpy} />
      </StoreProvider>,
    );

    const tabSwitch = screen.getByRole('combobox');

    expect(tabSwitch).toHaveTextContent('Test Results');
    fireEvent.mouseDown(tabSwitch);

    await act(() => jest.runOnlyPendingTimersAsync());
    fireEvent.click(screen.getByText('Timeline'));
    await act(() => jest.runOnlyPendingTimersAsync());

    expect(tabSwitch).toHaveTextContent('Timeline');

    expect(onCloseSpy.mock.calls.length).toStrictEqual(0);
    expect(setDefaultTabSpy.mock.calls.length).toStrictEqual(1);

    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    await act(() => jest.runOnlyPendingTimersAsync());

    expect(onCloseSpy.mock.calls.length).toStrictEqual(1);
    expect(setDefaultTabSpy.mock.calls.length).toStrictEqual(2);
    expect(store.userConfig.build.defaultTab).toStrictEqual('timeline');
  });

  it('should not update global config when dismissed', async () => {
    store.userConfig.build.setDefaultTab('test-results');
    const onCloseSpy = jest.fn();

    render(
      <StoreProvider value={store}>
        <ChangeConfigDialog open onClose={onCloseSpy} />
      </StoreProvider>,
    );

    const tabSwitch = screen.getByRole('combobox');
    expect(tabSwitch).toHaveTextContent('Test Results');

    fireEvent.mouseDown(tabSwitch);
    await act(() => jest.runOnlyPendingTimersAsync());
    fireEvent.click(screen.getByText('Timeline'));
    await act(() => jest.runOnlyPendingTimersAsync());

    expect(tabSwitch).toHaveTextContent('Timeline');

    expect(onCloseSpy.mock.calls.length).toStrictEqual(0);
    expect(setDefaultTabSpy.mock.calls.length).toStrictEqual(1);

    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
    await act(() => jest.runOnlyPendingTimersAsync());

    expect(onCloseSpy.mock.calls.length).toStrictEqual(1);
    expect(setDefaultTabSpy.mock.calls.length).toStrictEqual(1);
    expect(store.userConfig.build.defaultTab).toStrictEqual('test-results');
  });
});
