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

import { CollapsibleList } from '@/authdb/components/collapsible_list';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

describe('CollapsibleList', () => {
  const items = ['group1', 'group2'];

  test('if given items, displays them', async () => {
    render(
      <FakeContextProvider>
        <CollapsibleList items={items} title="test-list" />
      </FakeContextProvider>,
    );

    await screen.findByTestId('collapsible-list');
    expect(screen.getByText('test-list (2)')).toBeInTheDocument();
    for (const item of items) {
      expect(screen.getByText(item)).toBeInTheDocument();
    }
  });

  test('if given no items, displays "none" message', async () => {
    render(
      <FakeContextProvider>
        <CollapsibleList items={[]} title="test-list" />
      </FakeContextProvider>,
    );

    expect(screen.getByText('test-list (0)')).toBeInTheDocument();
    expect(screen.getByText('None')).toBeInTheDocument();
  });

  test('if variant is group-link, displays items as group links', async () => {
    render(
      <FakeContextProvider>
        <CollapsibleList items={items} title="test-list" variant="group-link" />
      </FakeContextProvider>,
    );

    await screen.findByTestId('collapsible-list');
    expect(screen.getByText('test-list (2)')).toBeInTheDocument();
    for (const item of items) {
      expect(screen.getByText(item)).toBeInTheDocument();
      expect(screen.getByRole('link', { name: item })).toHaveAttribute(
        'href',
        `/ui/auth/groups/${item}`,
      );
    }
  });

  test('if given numRedacted, redacts items in list', async () => {
    render(
      <FakeContextProvider>
        <CollapsibleList
          items={items}
          title="test-list"
          variant="group-link"
          numRedacted={5}
        />
      </FakeContextProvider>,
    );

    await screen.findByTestId('collapsible-list');
    expect(screen.getByText('test-list (7)')).toBeInTheDocument();
    expect(screen.getByText('5 members redacted.')).toBeInTheDocument();
    for (const item of items) {
      expect(screen.getByText(item)).toBeInTheDocument();
    }
  });

  test('if variant is principal-link, displays items as lookup links', async () => {
    render(
      <FakeContextProvider>
        <CollapsibleList
          items={items}
          title="test-list"
          variant="principal-link"
        />
      </FakeContextProvider>,
    );

    await screen.findByTestId('collapsible-list');
    expect(screen.getByText('test-list (2)')).toBeInTheDocument();
    for (const item of items) {
      expect(screen.getByText(item)).toBeInTheDocument();
      expect(screen.getByRole('link', { name: item })).toHaveAttribute(
        'href',
        `/ui/auth/lookup?p=${item}`,
      );
    }
  });
});
