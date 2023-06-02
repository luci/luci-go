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
import { cleanup, render } from '@testing-library/react';
import { destroy } from 'mobx-state-tree';

import { ANONYMOUS_IDENTITY } from '@/common/libs/auth_state';
import { Store, StoreInstance, StoreProvider } from '@/common/store';

import { BuilderList } from './builder_list';

describe('BuilderList', () => {
  let store: StoreInstance;

  beforeEach(() => {
    jest.useFakeTimers();
    store = Store.create({
      authState: { value: { identity: ANONYMOUS_IDENTITY } },
    });
  });
  afterEach(() => {
    cleanup();
    destroy(store);
    jest.useRealTimers();
  });

  it('should start loading all builders', () => {
    const loadRemainingPagesStub = jest.spyOn(
      store.searchPage.builderLoader!,
      'loadRemainingPages'
    );
    loadRemainingPagesStub.mockImplementation(() => Promise.resolve());

    render(
      <StoreProvider value={store}>
        <BuilderList />
      </StoreProvider>
    );

    jest.runOnlyPendingTimers();

    expect(loadRemainingPagesStub.mock.calls.length).toEqual(1);
  });
});
