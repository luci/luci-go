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

import { render } from '@testing-library/react';
import { Router } from '@vaadin/router';
import { expect } from 'chai';
import { destroy } from 'mobx-state-tree';
import * as sinon from 'sinon';

import { ANONYMOUS_IDENTITY } from '../services/milo_internal';
import { Store, StoreProvider } from '../store';
import { LoginPage } from './login_page';

describe('LoginPage', () => {
  let sandbox: sinon.SinonSandbox;
  beforeEach(() => {
    sandbox = sinon.createSandbox();
  });
  afterEach(() => {
    sandbox.restore();
  });

  it('should redirect properly', () => {
    const store = Store.create({ authState: { value: { identity: ANONYMOUS_IDENTITY } } });
    after(() => destroy(store));

    const routerGoStub = sandbox.stub(Router, 'go');

    const { rerender } = render(
      <StoreProvider value={store}>
        <LoginPage />
      </StoreProvider>
    );

    expect(routerGoStub.callCount).to.eq(0);

    store.authState.setValue({ identity: 'account@google.com' });
    rerender(
      <StoreProvider value={store}>
        <LoginPage />
      </StoreProvider>
    );

    expect(routerGoStub.callCount).to.eq(1);
  });
});
