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

import '@testing-library/jest-dom';

import { render, screen } from '@testing-library/react';
import { destroy } from 'mobx-state-tree';

import { Store, StoreInstance, StoreProvider } from '@/common/store';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { SettingsMenu } from './settings_menu';

describe('SettingsMenu', () => {
  let store: StoreInstance;

  beforeEach(() => {
    store = Store.create({});
  });
  afterEach(() => {
    destroy(store);
  });

  it('should display menu button', async () => {
    render(
      <StoreProvider value={store}>
        <FakeContextProvider>
          <SettingsMenu />
        </FakeContextProvider>
      </StoreProvider>
    );
    await screen.findAllByRole('button');
    expect(screen.getByLabelText('open settings menu')).toBeInTheDocument();
  });
});
