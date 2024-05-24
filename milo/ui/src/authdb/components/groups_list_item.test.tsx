// Copyright 2024 The LUCI Authors.
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

import { createMockGroup } from '@/authdb/testing_tools/mocks/group_mock';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import List from '@mui/material/List';

import { GroupsListItem } from './groups_list_item';

describe('<GroupsListItem />', () => {
  test('if groups name & description is displayed', async () => {
    const mockGroup = createMockGroup('123');

    render(
      <FakeContextProvider>
        <List>
            <GroupsListItem group={mockGroup} setSelected={() => {}} selected={false}/>
        </List>
      </FakeContextProvider>,
    );

    await screen.findByTestId('groups_item_list_item_text');

    // Check both name and description is displayed.
    expect(screen.getByText(mockGroup.name)).toBeInTheDocument();
    expect(screen.getByText(mockGroup.description)).toBeInTheDocument();
  });
});
