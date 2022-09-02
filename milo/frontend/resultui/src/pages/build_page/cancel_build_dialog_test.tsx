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
import { BuildPageInstance } from '../../store/build_page';
import { CancelBuildDialog } from './cancel_build_dialog';

describe('CancelBuildDialog', () => {
  let timer: sinon.SinonFakeTimers;
  let store: Instance<typeof Store>;
  let cancelBuildStub: sinon.SinonStub<
    Parameters<BuildPageInstance['cancelBuild']>,
    ReturnType<BuildPageInstance['cancelBuild']>
  >;
  beforeEach(() => {
    timer = sinon.useFakeTimers();
    store = Store.create();
    unprotect(store);
    cancelBuildStub = sinon.stub(store.buildPage!, 'cancelBuild');
    protect(store);
    cancelBuildStub.resolves();
  });

  afterEach(() => {
    timer.restore();
    destroy(store);
  });

  it('should not trigger cancel request when reason is not provided', async () => {
    const onCloseSpy = sinon.spy();

    render(
      <StoreProvider value={store}>
        <CancelBuildDialog open onClose={onCloseSpy} />
      </StoreProvider>
    );

    fireEvent.click(screen.getByText('Confirm'));
    await timer.runToLastAsync();
    expect(onCloseSpy.callCount).to.eq(0);
    expect(cancelBuildStub.callCount).to.eq(0);
    screen.getByText('Reason is required');
  });

  it('should trigger cancel request when reason is provided', async () => {
    const onCloseSpy = sinon.spy();

    render(
      <StoreProvider value={store}>
        <CancelBuildDialog open onClose={onCloseSpy} />
      </StoreProvider>
    );

    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'need to stop build' } });
    fireEvent.click(screen.getByText('Confirm'));
    await timer.runToLastAsync();
    expect(onCloseSpy.callCount).to.eq(1);
    expect(screen.queryByText('Reason is required')).to.be.null;
    expect(cancelBuildStub.callCount).to.eq(1);
    expect(cancelBuildStub.getCall(0).args[0]).to.eq('need to stop build');
  });
});
