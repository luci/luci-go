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

import {
  createMockPrincipalPermissions,
  mockFetchGetPrincipalPermissions,
  mockErrorFetchingGetPermissions,
} from '@/authdb/testing_tools/mocks/group_permissions_mock';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { GroupPermissions } from './group_permissions';

describe('<GroupPermissions />', () => {
  test('displays realm names', async () => {
    const mockPermissions = createMockPrincipalPermissions('123');
    mockFetchGetPrincipalPermissions(mockPermissions);

    render(
      <FakeContextProvider>
        <GroupPermissions name="123" />
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-permissions');

    for (const realmPermission of mockPermissions.realmPermissions) {
      expect(screen.getByText(realmPermission.name)).toBeInTheDocument();
    }
  });

  test('displays permissions', async () => {
    const mockPermissions = createMockPrincipalPermissions('123');
    mockFetchGetPrincipalPermissions(mockPermissions);

    render(
      <FakeContextProvider>
        <GroupPermissions name="123" />
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-permissions');

    for (const realmPermission of mockPermissions.realmPermissions) {
      for (const permission of realmPermission.permissions) {
        expect(screen.getByText(permission)).toBeInTheDocument();
      }
    }
  });

  test('if appropriate message is displayed for an error', async () => {
    mockErrorFetchingGetPermissions();

    render(
      <FakeContextProvider>
        <GroupPermissions name="123" />
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-permissions-error');

    expect(
      screen.getByText('Failed to load group permissions'),
    ).toBeInTheDocument();
    expect(screen.queryByTestId('group-permissions')).toBeNull();
  });
});
