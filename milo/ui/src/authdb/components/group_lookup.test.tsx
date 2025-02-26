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
  createMockSubgraph,
  mockErrorFetchingGetSubgraph,
  mockFetchGetSubgraph,
} from '@/authdb/testing_tools/mocks/group_subgraph_mock';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { GroupLookup, interpretLookupResults } from './group_lookup';

describe('<GroupLookup />', () => {
  test('interprets lookup results correctly', async () => {
    const mockSubgraph = createMockSubgraph('requestedGroup');
    const interpretedGraph = interpretLookupResults(mockSubgraph);

    const expectedDirectIncluders = [
      {
        name: 'nestedGroup',
        includesDirectly: true,
        includesViaGlobs: [],
        includesIndirectly: [],
      },
    ];
    const expectedIndirectIncluders = [
      {
        name: 'nestedGroup2',
        includesDirectly: false,
        includesViaGlobs: [],
        includesIndirectly: [['nestedGroup']],
      },
      {
        name: 'owningGroup',
        includesDirectly: false,
        includesViaGlobs: [],
        includesIndirectly: [['nestedGroup', 'nestedGroup2']],
      },
    ];
    expect(interpretedGraph).not.toBeNull();
    expect(interpretedGraph.directIncluders).toEqual(expectedDirectIncluders);
    expect(interpretedGraph.indirectIncluders).toEqual(
      expectedIndirectIncluders,
    );
  });

  test('displays lookup results', async () => {
    const mockSubgraph = createMockSubgraph('requestedGroup');
    mockFetchGetSubgraph(mockSubgraph);

    render(
      <FakeContextProvider>
        <GroupLookup name="requestedGroup" />
      </FakeContextProvider>,
    );
    await screen.findByTestId('lookup-table');
    const groups = ['nestedGroup', 'nestedGroup2', 'owningGroup'];
    groups.forEach((group) => {
      expect(screen.getByText(group)).toBeInTheDocument();
    });
  });

  test('groups have correct links', async () => {
    const mockSubgraph = createMockSubgraph('requestedGroup');
    mockFetchGetSubgraph(mockSubgraph);

    render(
      <FakeContextProvider>
        <GroupLookup name="requestedGroup" />
      </FakeContextProvider>,
    );
    await screen.findByTestId('lookup-table');

    const groups = ['nestedGroup', 'nestedGroup2', 'owningGroup'];
    groups.forEach((group) => {
      expect(screen.getByText(group)).toBeInTheDocument();
      expect(screen.getByTestId(`${group}-link`)).toHaveAttribute(
        'href',
        `/ui/auth/groups/${group}`,
      );
    });
  });

  test('if appropriate message is displayed for an error', async () => {
    mockErrorFetchingGetSubgraph();

    render(
      <FakeContextProvider>
        <GroupLookup name="123" />
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-lookup-error');

    expect(
      screen.getByText('Failed to load group ancestors'),
    ).toBeInTheDocument();
    expect(screen.queryByTestId('lookup-table')).toBeNull();
  });
});
