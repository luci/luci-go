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

import { render, screen, fireEvent } from '@testing-library/react';

import { createMockGroupIndividual } from '@/authdb/testing_tools/mocks/group_individual_mock';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { GroupsFormListReadonly } from './groups_form_list_readonly';

describe('<GroupsFormList readonly/>', () => {
  const mockGroup = createMockGroupIndividual('123', false, true);
  beforeEach(async () => {
    render(
      <FakeContextProvider>
        <GroupsFormListReadonly
          name="Members"
          initialItems={mockGroup.members as string[]}
        />
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form-list-readonly');
  });
  test('does not show add button', async () => {
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).toBeNull();
  });

  test('does not show remove button on hover', async () => {
    // Simulate mouse enter event each row.
    for (let i = 0; i < mockGroup.members.length; i++) {
      const row = screen.getByTestId(`item-row-${mockGroup.members[i]}`);
      fireEvent.mouseEnter(row);
      expect(
        screen.queryByTestId(`remove-button-${mockGroup.members[i]}`),
      ).toBeNull();
    }
  });
});
