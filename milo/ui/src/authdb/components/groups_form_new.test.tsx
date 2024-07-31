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

import { act } from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import List from '@mui/material/List';
import { GroupsFormNew } from './groups_form_new';

describe('<GroupsFormNew />', () => {
  test('if group name, description textarea is displayed', async () => {

    render(
      <FakeContextProvider>
        <List>
            <GroupsFormNew/>
        </List>
      </FakeContextProvider>,
    );

    await screen.findByTestId('groups-form-new');
    expect(screen.getByTestId('name-textfield')).toBeInTheDocument();
    expect(screen.getByTestId('description-textfield')).toBeInTheDocument();
  });
  test('valid form shows no errors', async () => {

    render(
      <FakeContextProvider>
        <List>
            <GroupsFormNew/>
        </List>
      </FakeContextProvider>,
    );

    await screen.findByTestId('groups-form-new');
    const nameTextfield = screen.getByTestId('name-textfield').querySelector('input');
    const ownersTextfield = screen.getByTestId('owners-textfield').querySelector('input');
    const descriptionTextfield = screen.getByTestId('description-textfield').querySelector('input');
    const membersTextfield = screen.getByTestId('members-textfield').querySelector('textarea');

    fireEvent.change(nameTextfield!, {target: { value: 'name'}});
    fireEvent.change(ownersTextfield!, {target: { value: 'administrators'}});
    fireEvent.change(descriptionTextfield!, {target: { value: 'test group'}});
    fireEvent.change(membersTextfield!, {target: { value: 'name-name@email.com'}});

    const createButton = screen.getByTestId('create-button');
    act(() => createButton.click());
    expect(screen.queryByText('Invalid group name.')).toBeNull();
    expect(screen.queryByText('Description is required.')).toBeNull();
    expect(screen.queryByText('Invalid owners name. Must be a group.')).toBeNull();
    expect(screen.queryByText('Invalid members:', {exact: false})).toBeNull();

});


  test('error is shown if creating invalid name', async () => {

    render(
      <FakeContextProvider>
        <List>
            <GroupsFormNew/>
        </List>
      </FakeContextProvider>,
    );

    await screen.findByTestId('groups-form-new');
    const textfield = screen.getByTestId('name-textfield').querySelector('input');
    fireEvent.change(textfield!, {target: { value: 'Invalid name'}});
    const createButton = screen.getByTestId('create-button');
    act(() => createButton.click());
    expect(screen.getByText('Invalid group name.')).toBeInTheDocument();
  });

  test('error is shown on empty description', async () => {

    render(
      <FakeContextProvider>
        <List>
            <GroupsFormNew/>
        </List>
      </FakeContextProvider>,
    );

    await screen.findByTestId('groups-form-new');
    const createButton = screen.getByTestId('create-button');
    act(() => createButton.click());
    expect(screen.getByText('Description is required.')).toBeInTheDocument();
  });

  test('error is shown on invalid owners name', async () => {

    render(
      <FakeContextProvider>
        <List>
            <GroupsFormNew/>
        </List>
      </FakeContextProvider>,
    );

    await screen.findByTestId('groups-form-new');
    const textfield = screen.getByTestId('owners-textfield').querySelector('input');
    fireEvent.change(textfield!, {target: { value: 'Invalid owners'}});
    const createButton = screen.getByTestId('create-button');
    act(() => createButton.click());
    expect(screen.getByText('Invalid owners name. Must be a group.')).toBeInTheDocument();
  });

  test('error is shown on invalid members name', async () => {

    render(
      <FakeContextProvider>
        <List>
            <GroupsFormNew/>
        </List>
      </FakeContextProvider>,
    );

    await screen.findByTestId('groups-form-new');
    const textfield = screen.getByTestId('members-textfield').querySelector('textarea');
    fireEvent.change(textfield!, {target: { value: '!@email.com'}});
    const createButton = screen.getByTestId('create-button');
    act(() => createButton.click());
    expect(screen.getByText('Invalid members: !@email.com')).toBeInTheDocument();
  });
});
