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

import { render } from '@testing-library/react';
import { expect } from 'chai';
import { destroy } from 'mobx-state-tree';
import * as sinon from 'sinon';

import { ANONYMOUS_IDENTITY } from '../../services/milo_internal';
import { Store, StoreProvider } from '../../store';
import { BuilderList } from './builder_list';

describe('BuilderList', () => {
  let timer: sinon.SinonFakeTimers;
  beforeEach(() => {
    timer = sinon.useFakeTimers();
  });
  afterEach(() => timer.restore());

  it('should start loading all builders', () => {
    const store = Store.create({ authState: { value: { identity: ANONYMOUS_IDENTITY } } });
    after(() => destroy(store));
    timer.runAll();
    const loadRemainingPagesStub = sinon.stub(store.searchPage.builderLoader!, 'loadRemainingPages');

    render(
      <StoreProvider value={store}>
        <BuilderList />
      </StoreProvider>
    );

    timer.runAll();

    expect(loadRemainingPagesStub.callCount).to.eq(1);
  });
});
