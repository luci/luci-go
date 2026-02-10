// Copyright 2026 The LUCI Authors.
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

import { render, screen } from '@testing-library/react';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ShortcutProvider } from '../shortcut_provider';

import { ColumnsButton } from './columns_button';

describe('<ColumnsButton />', () => {
  it('renders "Columns" text by default', () => {
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <ColumnsButton
            allColumns={[]}
            visibleColumns={[]}
            onToggleColumn={() => {}}
          />
        </ShortcutProvider>
      </FakeContextProvider>,
    );
    expect(screen.getByText('Columns')).toBeVisible();
  });

  it('renders custom trigger if provided', () => {
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <ColumnsButton
            allColumns={[]}
            visibleColumns={[]}
            onToggleColumn={() => {}}
            renderTrigger={(props, ref) => (
              <button ref={ref} onClick={props.onClick}>
                Custom
              </button>
            )}
          />
        </ShortcutProvider>
      </FakeContextProvider>,
    );
    expect(screen.getByText('Custom')).toBeVisible();
    expect(screen.queryByText('Columns')).not.toBeInTheDocument();
  });
});
