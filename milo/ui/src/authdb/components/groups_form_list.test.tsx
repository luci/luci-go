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
import userEvent from '@testing-library/user-event'
import { createMockGroupIndividual } from '@/authdb/testing_tools/mocks/group_individual_mock';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { GroupsFormList } from './groups_form_list';
import { act } from 'react';

describe('<GroupsFormList />', () => {
    const mockGroup = createMockGroupIndividual('123');
    beforeEach(async () => {
        render(
          <FakeContextProvider>
            <GroupsFormList name='Members' initialItems={mockGroup.members as string[]}/>
          </FakeContextProvider>,
        );
        await screen.findByTestId('groups-form-list');
    });

    test('displays items in readonly mode', async () => {
        // Check each member is displayed.
        for (let i = 0; i < mockGroup.members.length; i++) {
            expect(screen.getByText(mockGroup.members[i])).toBeInTheDocument();
        }
        // Check no remove button exists on readonly.
        expect(screen.queryAllByTestId('remove-button')).toHaveLength(0);
    });

  test('can be toggled to edit mode', async () => {
    // Check edit button exists.
    const button = screen.queryByTestId('edit-button');
    expect(button).not.toBeNull();
    act(() => button!.click());
    // Check that remove button exists for each member.
    for (let i = 0; i < mockGroup.members.length; i++) {
        expect(screen.getByTestId(`remove-button-${mockGroup.members[i]}`)).not.toBeNull();
    }
    const addButton = screen.queryByTestId('add-button');
    // Check add button appears on edit.
    expect(addButton).not.toBeNull();
  })

  test('can remove members', async () => {
    const button = screen.queryByTestId('edit-button');
    expect(button).not.toBeNull();
    act(() => button!.click());
    // Expect remove button to remove member from list.
    for (let i = 0; i < mockGroup.members.length; i++) {
        const removeButton = screen.getByTestId(`remove-button-${mockGroup.members[i]}`)
        act(() => removeButton.click());
        expect(screen.queryByText(mockGroup.members[i])).toBeNull();
    }
  })

  test('can add members', async () => {
    // Click edit button.
    const button = screen.queryByTestId('edit-button');
    expect(button).not.toBeNull();
    act(() => button!.click());
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen.getByTestId('add-textfield').querySelector('input');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, 'newMember');
    expect(textfield!.value).toBe('newMember');
    // Click confirm button.
    const confirmButton = screen.queryByTestId('confirm-button');
    expect(confirmButton).not.toBeNull();
    act(() => confirmButton!.click());
    // Check new member shown in list?
    expect(screen.getByText('newMember')).toBeInTheDocument();
  })

  test('can add members with enter button', async () => {
    // Click edit button.
    const button = screen.queryByTestId('edit-button');
    expect(button).not.toBeNull();
    act(() => button!.click());
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen.getByTestId('add-textfield').querySelector('input');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, 'newMember');
    expect(textfield!.value).toBe('newMember');
    // Press enter on textfield.
    fireEvent.keyDown(textfield!, {key: 'Enter', code: 'Enter', charCode: 13})
    // Check new member shown in list.
    expect(screen.getByText('newMember')).toBeInTheDocument();
  })

  test('clears textfield with clear button', async() => {
    // Click edit button.
    const button = screen.queryByTestId('edit-button');
    expect(button).not.toBeNull();
    act(() => button!.click());
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen.getByTestId('add-textfield').querySelector('input');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, 'newMember');
    expect(textfield!.value).toBe('newMember');
    // Click clear button.
    const confirmButton = screen.queryByTestId('confirm-button');
    expect(confirmButton).not.toBeNull();
    act(() => confirmButton!.click());
    // CHeck textfield is empty.
    expect(textfield!.value).toBe('');
  })
});
