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

import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { VirtuosoMockContext } from 'react-virtuoso';

import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ArtifactTreeView } from './artifact_tree_view';

describe('<ArtifactTreeView />', () => {
  const resultArtifacts: readonly Artifact[] = [
    Artifact.fromPartial({
      artifactId: 'artifact1.log',
      name: 'artifacts/artifact1.log',
    }),
    Artifact.fromPartial({
      artifactId: 'artifact2.png',
      name: 'artifacts/artifact2.png',
    }),
  ];

  const invArtifacts: readonly Artifact[] = [
    Artifact.fromPartial({
      artifactId: 'invArtifact1.log',
      name: 'invArtifacts/invArtifact1.log',
    }),
    Artifact.fromPartial({
      artifactId: 'invArtifact2.jpg',
      name: 'invArtifacts/invArtifact2.jpg',
    }),
  ];

  it('given an list of artifacts, should create a tree out of their names', async () => {
    render(
      <VirtuosoMockContext.Provider
        value={{ viewportHeight: 300, itemHeight: 30 }}
      >
        <FakeContextProvider>
          <ArtifactTreeView
            resultArtifacts={resultArtifacts}
            invArtifacts={invArtifacts}
            artifactsLoading={false}
            updateSelectedArtifact={() => {}}
          />
        </FakeContextProvider>
      </VirtuosoMockContext.Provider>,
    );

    await screen.findByText('Result artifacts');

    expect(screen.getByText('Invocation artifacts')).toBeInTheDocument();
    expect(screen.getByText('artifact1.log')).toBeInTheDocument();
    expect(screen.getByText('artifact2.png')).toBeInTheDocument();
    expect(screen.getByText('invArtifact1.log')).toBeInTheDocument();
    expect(screen.getByText('invArtifact2.jpg')).toBeInTheDocument();
  });

  it('should filter the artifact tree when a user types in the search box', async () => {
    jest.useFakeTimers();
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime });

    render(
      <VirtuosoMockContext.Provider
        value={{ viewportHeight: 300, itemHeight: 30 }}
      >
        <FakeContextProvider>
          <ArtifactTreeView
            resultArtifacts={resultArtifacts}
            invArtifacts={invArtifacts}
            artifactsLoading={false}
            updateSelectedArtifact={() => {}}
          />
        </FakeContextProvider>
      </VirtuosoMockContext.Provider>,
    );

    const searchBox = screen.getByPlaceholderText('Search artifacts');
    await user.type(searchBox, 'log');

    await act(async () => {
      jest.runAllTimers();
    });

    // Assert that non-matching artifacts are hidden.
    expect(screen.queryByText('artifact2.png')).not.toBeInTheDocument();
    expect(screen.queryByText('invArtifact2.jpg')).not.toBeInTheDocument();

    // Find all rendered tree items.
    const treeItems = screen.getAllByRole('treeitem');

    // Filter to get only the items for our log files. This is more specific
    // than a broad search for the text 'log'.
    const logFileItems = treeItems.filter((item) =>
      item.textContent?.endsWith('.log'),
    );

    // We should have exactly two log files remaining.
    expect(logFileItems).toHaveLength(2);

    expect(logFileItems[0]).toHaveTextContent('artifact1.log');

    expect(logFileItems[1]).toHaveTextContent('invArtifact1.log');

    jest.useRealTimers();
  });
});
