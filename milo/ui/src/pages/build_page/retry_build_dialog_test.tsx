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
import { Router } from '@vaadin/router';
import { expect } from 'chai';
import { destroy, Instance, protect, unprotect } from 'mobx-state-tree';
import * as sinon from 'sinon';

import { Build } from '../../services/buildbucket';
import { Store, StoreProvider } from '../../store';
import { BuildPageInstance } from '../../store/build_page';
import { RetryBuildDialog } from './retry_build_dialog';

describe('RetryBuildDialog', () => {
  let sandbox: sinon.SinonSandbox;
  let timer: sinon.SinonFakeTimers;
  let store: Instance<typeof Store>;
  let retryBuildStub: sinon.SinonStub<
    Parameters<BuildPageInstance['retryBuild']>,
    ReturnType<BuildPageInstance['retryBuild']>
  >;
  beforeEach(() => {
    sandbox = sinon.createSandbox();
    timer = sandbox.useFakeTimers();
    store = Store.create();
    unprotect(store);
    retryBuildStub = sandbox.stub(store.buildPage!, 'retryBuild');
    protect(store);
    retryBuildStub.resolves({
      builder: { project: 'proj', bucket: 'bucket', builder: 'builder' },
      number: 123,
    } as Build);
  });

  afterEach(() => {
    unprotect(store);
    sandbox.restore();
    protect(store);
    destroy(store);
  });

  it('should redirect to the new build', async () => {
    const onCloseSpy = sandbox.spy();
    const routerGoStub = sandbox.stub(Router, 'go');

    render(
      <StoreProvider value={store}>
        <RetryBuildDialog open onClose={onCloseSpy} />
      </StoreProvider>
    );

    fireEvent.click(screen.getByText('Confirm'));
    await timer.runToLastAsync();
    expect(onCloseSpy.callCount).to.eq(1);
    expect(retryBuildStub.callCount).to.eq(1);
    expect(routerGoStub.callCount).to.eq(1);
    expect(routerGoStub.getCall(0).args[0]).to.eq('/ui/p/proj/builders/bucket/builder/123');
  });
});
