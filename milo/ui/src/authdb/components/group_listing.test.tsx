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
  createMockExpandedGroup,
  mockFetchGetExpandedGroup,
  mockErrorFetchingGetExpandedGroup,
} from '@/authdb/testing_tools/mocks/group_expanded_mock';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { GroupListing } from './group_listing';

describe('<GroupListing />', () => {
  test('displays group members', async () => {
    const mockGroup = createMockExpandedGroup('123');
    mockFetchGetExpandedGroup(mockGroup);

    render(
      <FakeContextProvider>
        <GroupListing name="123" />
      </FakeContextProvider>,
    );

    await screen.findByTestId('listing-table');
    expect(screen.getByText(mockGroup.members[0])).toBeInTheDocument();
    expect(screen.getByText(mockGroup.members[1])).toBeInTheDocument();
  });
  test('displays group globs', async () => {
    const mockGroup = createMockExpandedGroup('123');
    mockFetchGetExpandedGroup(mockGroup);

    render(
      <FakeContextProvider>
        <GroupListing name="123" />
      </FakeContextProvider>,
    );

    await screen.findByTestId('listing-table');
    expect(screen.getByText(mockGroup.globs[0])).toBeInTheDocument();
  });

  test('displays nested groups', async () => {
    const mockGroup = createMockExpandedGroup('123');
    mockFetchGetExpandedGroup(mockGroup);

    render(
      <FakeContextProvider>
        <GroupListing name="123" />
      </FakeContextProvider>,
    );

    await screen.findByTestId('listing-table');
    expect(screen.getByText(mockGroup.nested[0])).toBeInTheDocument();
    expect(screen.getByText(mockGroup.nested[1])).toBeInTheDocument();
  });

  test('if appropriate message is displayed for an error', async () => {
    mockErrorFetchingGetExpandedGroup();

    render(
      <FakeContextProvider>
        <GroupListing name="123" />
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-listing-error');

    expect(
      screen.getByText('Failed to load group listing'),
    ).toBeInTheDocument();
    expect(screen.queryByTestId('listing-table')).toBeNull();
  });
});
