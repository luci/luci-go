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

import { Component as GroupsPageWrapper } from '@/authdb/pages/groups_page';
import {
  createMockGroupIndividual,
  mockFetchGetGroup,
} from '@/authdb/testing_tools/mocks/group_individual_mock';
import { mockFetchGroups } from '@/authdb/testing_tools/mocks/groups_list_mock';
import { AuthGroup } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

describe('<GroupsPage />', () => {
  test('Updates document title based on route param', async () => {
    const mockGroupsList = [
      AuthGroup.fromPartial({ name: 'administrators' }),
      AuthGroup.fromPartial({ name: 'test-group' }),
    ];
    mockFetchGroups(mockGroupsList);

    const mockAdminGroup = createMockGroupIndividual(
      'administrators',
      true,
      true,
    );
    mockFetchGetGroup(mockAdminGroup);

    render(
      <FakeContextProvider
        mountedPath="/groups/*"
        routerOptions={{
          initialEntries: ['/groups/administrators'],
        }}
      >
        <GroupsPageWrapper />
      </FakeContextProvider>,
    );

    // Wait for the group details to load to ensure the component has settled.
    await screen.findByText('administrators');
    expect(document.title).toBe('administrators | LUCI');
  });

  test('Updates document title to Create Group when at new!', async () => {
    const mockGroupsList = [AuthGroup.fromPartial({ name: 'administrators' })];
    mockFetchGroups(mockGroupsList);

    render(
      <FakeContextProvider
        mountedPath="/groups/*"
        routerOptions={{
          initialEntries: ['/groups/new!'],
        }}
      >
        <GroupsPageWrapper />
      </FakeContextProvider>,
    );

    await screen.findByText('Create new group');
    expect(document.title).toBe('Create new group | LUCI');
  });
});
