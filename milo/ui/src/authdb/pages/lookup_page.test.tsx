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

import { render, screen, fireEvent } from '@testing-library/react';
import { act } from 'react';

import {
  createMockPrincipalPermissions,
  mockFetchGetPrincipalPermissions,
} from '@/authdb/testing_tools/mocks/group_permissions_mock';
import {
  createMockSubgraph,
  mockFetchGetSubgraph,
} from '@/authdb/testing_tools/mocks/group_subgraph_mock';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { LookupPage } from './lookup_page';

describe('<LookupPage />', () => {
  test('Pressing search button with valid query displays lookup results', async () => {
    const mockSubgraph = createMockSubgraph('requestedGroup');
    mockFetchGetSubgraph(mockSubgraph);

    render(
      <FakeContextProvider>
        <LookupPage />
      </FakeContextProvider>,
    );
    const textfield = screen
      .getByTestId('lookup-textfield')
      .querySelector('input');
    fireEvent.change(textfield!, { target: { value: 'requestedGroup' } });
    const searchButton = screen.getByTestId('search-button');
    act(() => searchButton.click());
    await screen.findByTestId('lookup-table');
    mockSubgraph.nodes.forEach((node) => {
      // Exclude requested group itself.
      if (node!.principal!.name === 'requestedGroup') {
        return;
      }
      expect(screen.getByText(node!.principal!.name)).toBeInTheDocument();
    });
  });
  test('Pressing enter in textfield with valid query displays lookup results', async () => {
    const mockSubgraph = createMockSubgraph('requestedGroup');
    mockFetchGetSubgraph(mockSubgraph);

    render(
      <FakeContextProvider>
        <LookupPage />
      </FakeContextProvider>,
    );
    const textfield = screen
      .getByTestId('lookup-textfield')
      .querySelector('input');
    fireEvent.change(textfield!, { target: { value: 'requestedGroup' } });
    fireEvent.keyDown(textfield!, {
      key: 'Enter',
      code: 'Enter',
      charCode: 13,
    });
    await screen.findByTestId('lookup-table');
    mockSubgraph.nodes.forEach((node) => {
      // Exclude requested group itself.
      if (node!.principal!.name === 'requestedGroup') {
        return;
      }
      expect(screen.getByText(node!.principal!.name)).toBeInTheDocument();
    });
  });
  test('Navigates to permission tab to display realms permissions', async () => {
    const mockPermissions =
      createMockPrincipalPermissions('requestedPrincipal');
    mockFetchGetPrincipalPermissions(mockPermissions);

    render(
      <FakeContextProvider>
        <LookupPage />
      </FakeContextProvider>,
    );
    const textfield = screen
      .getByTestId('lookup-textfield')
      .querySelector('input');
    fireEvent.change(textfield!, { target: { value: 'requestedPrincipal' } });
    const searchButton = screen.getByTestId('search-button');
    act(() => searchButton.click());

    // Click the permissions tab now that something has been searched.
    const permissionsTab = await screen.findByTestId('permissions-tab');
    act(() => permissionsTab.click());

    await screen.findByTestId('permissions-table');
    for (const realmPermission of mockPermissions.realmPermissions) {
      for (const permission of realmPermission.permissions) {
        expect(screen.getByText(permission)).toBeInTheDocument();
      }
    }
  });
});
