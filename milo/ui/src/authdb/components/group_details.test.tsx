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

import List from '@mui/material/List';
import { render, screen } from '@testing-library/react';

import { GroupDetails } from '@/authdb/components/group_details';
import {
  createMockGroupIndividual,
  mockFetchGetGroup,
} from '@/authdb/testing_tools/mocks/group_individual_mock';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

describe('<GroupDetails />', () => {
  test('displays group name', async () => {
    const mockGroup = createMockGroupIndividual('123', true, true);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <List>
          <GroupDetails name="123" refetchList={() => {}} />
        </List>
      </FakeContextProvider>,
    );

    expect(screen.getByText(mockGroup.name)).toBeInTheDocument();
  });

  test('displays config link for mdb/ group', async () => {
    const mockGroup = createMockGroupIndividual('mdb/foo', true, true);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <List>
          <GroupDetails name="mdb/foo" refetchList={() => {}} />
        </List>
      </FakeContextProvider>,
    );

    expect(screen.getByText('mdb/foo')).toBeInTheDocument();
    const link = screen.getByRole('link', { name: '[config]' });
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute(
      'href',
      'http://cs/f:groups_push_cron/config.yaml%20foo',
    );
  });

  test('displays config link for google/ group', async () => {
    const mockGroup = createMockGroupIndividual('google/bar', true, true);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <List>
          <GroupDetails name="google/bar" refetchList={() => {}} />
        </List>
      </FakeContextProvider>,
    );

    expect(screen.getByText('google/bar')).toBeInTheDocument();
    const link = screen.getByRole('link', { name: '[config]' });
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute(
      'href',
      'http://cs/f:groups_push_cron/config.yaml%20bar',
    );
  });

  test('does not display config link for regular group', async () => {
    const mockGroup = createMockGroupIndividual('regular-group', true, true);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <List>
          <GroupDetails name="regular-group" refetchList={() => {}} />
        </List>
      </FakeContextProvider>,
    );

    expect(screen.getByText('regular-group')).toBeInTheDocument();
    expect(
      screen.queryByRole('link', { name: '[config]' }),
    ).not.toBeInTheDocument();
  });
});
