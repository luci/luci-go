// Copyright 2025 The LUCI Authors.
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

import { CollapsibleList } from './collapsible_list';

describe('<CollapsibleList />', () => {
  const items = ['group1', 'group2'];

  test('displays items', async () => {
    render(
      <FakeContextProvider>
        <CollapsibleList
          items={items}
          renderAsGroupLinks={false}
          title="test-list"
        />
      </FakeContextProvider>,
    );

    await screen.findByTestId('collapsible-list');
    for (const item of items) {
      expect(screen.getByText(item)).toBeInTheDocument();
    }
  });

  test('displays items as links', async () => {
    render(
      <FakeContextProvider>
        <CollapsibleList
          items={items}
          renderAsGroupLinks={true}
          title="test-list"
        />
      </FakeContextProvider>,
    );

    await screen.findByTestId('collapsible-list');
    for (const item of items) {
      expect(screen.getByText(item)).toBeInTheDocument();
      expect(screen.getByTestId(`${item}-link`)).toHaveAttribute(
        'href',
        `/ui/auth/groups/${item}`,
      );
    }
  });

  test('redacts items in list', async () => {
    render(
      <FakeContextProvider>
        <CollapsibleList
          items={items}
          renderAsGroupLinks={true}
          title="test-list"
          numRedacted={5}
        />
      </FakeContextProvider>,
    );

    await screen.findByTestId('collapsible-list');
    expect(screen.getByText('5 members redacted.')).toBeInTheDocument();
    for (const item of items) {
      expect(screen.getByText(item)).toBeInTheDocument();
    }
  });
});
