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

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { destroy, Instance, protect, unprotect } from 'mobx-state-tree';
import { act } from 'react-dom/test-utils';

import { Store, StoreProvider } from '@/common/store';

import { CancelBuildDialog } from './cancel_build_dialog';

describe('CancelBuildDialog', () => {
  let store: Instance<typeof Store>;
  let cancelBuildStub: jest.SpiedFunction<(reason: string) => Promise<void>>;
  beforeEach(() => {
    jest.useFakeTimers();
    store = Store.create();
    unprotect(store);
    cancelBuildStub = jest.spyOn(store.buildPage!, 'cancelBuild');
    protect(store);
    cancelBuildStub.mockResolvedValue();
  });

  afterEach(() => {
    cleanup();
    destroy(store);
    jest.useRealTimers();
  });

  test('should not trigger cancel request when reason is not provided', async () => {
    const onCloseSpy = jest.fn();

    render(
      <StoreProvider value={store}>
        <CancelBuildDialog open onClose={onCloseSpy} />
      </StoreProvider>,
    );

    fireEvent.click(screen.getByText('Confirm'));
    await act(async () => {
      await jest.runOnlyPendingTimersAsync();
    });
    expect(onCloseSpy.mock.calls.length).toStrictEqual(0);
    expect(cancelBuildStub.mock.calls.length).toStrictEqual(0);
    screen.getByText('Reason is required');
  });

  test('should trigger cancel request when reason is provided', async () => {
    const onCloseSpy = jest.fn();

    render(
      <StoreProvider value={store}>
        <CancelBuildDialog open onClose={onCloseSpy} />
      </StoreProvider>,
    );

    fireEvent.change(screen.getByRole('textbox'), {
      target: { value: 'need to stop build' },
    });
    fireEvent.click(screen.getByText('Confirm'));
    await act(async () => {
      await jest.runOnlyPendingTimersAsync();
    });
    expect(onCloseSpy.mock.calls.length).toStrictEqual(1);
    expect(screen.queryByText('Reason is required')).toBeNull();
    expect(cancelBuildStub.mock.calls.length).toStrictEqual(1);
    expect(cancelBuildStub.mock.lastCall?.[0]).toStrictEqual(
      'need to stop build',
    );
  });
});
