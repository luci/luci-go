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
import { TestResultBundle } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ArtifactsProvider } from '../context';

import { ArtifactTreeView } from './artifact_tree_view';

// Mock window.open for the JSDOM environment
window.open = jest.fn();

describe('<ArtifactTreeView />', () => {
  const resultArtifacts: readonly Artifact[] = [
    Artifact.fromPartial({
      artifactId: 'artifact1.log',
      name: 'invocations/inv/artifacts/artifact1.log',
      hasLines: true, // This artifact is viewable in the app
    }),
    Artifact.fromPartial({
      artifactId: 'artifact2.png',
      name: 'invocations/inv/artifacts/artifact2.png',
      // This artifact is not viewable, clicking it will call window.open
    }),
  ];

  const invArtifacts: readonly Artifact[] = [
    Artifact.fromPartial({
      artifactId: 'invArtifact1.log',
      name: 'invocations/inv/artifacts/invArtifact1.log',
      hasLines: true, // This artifact is viewable in the app
    }),
    Artifact.fromPartial({
      artifactId: 'invArtifact2.jpg',
      name: 'invocations/inv/artifacts/invArtifact2.jpg',
    }),
  ];

  const MOCK_RESULTS: readonly TestResultBundle[] = [];

  it('should create a tree and call updateSelectedArtifact when a node is clicked', async () => {
    const user = userEvent.setup();
    const updateSelectedArtifact = jest.fn();

    render(
      <VirtuosoMockContext.Provider
        value={{ viewportHeight: 300, itemHeight: 30 }}
      >
        <FakeContextProvider>
          <ArtifactsProvider results={MOCK_RESULTS}>
            <ArtifactTreeView
              resultArtifacts={resultArtifacts}
              invArtifacts={invArtifacts}
              artifactsLoading={false}
              updateSelectedArtifact={updateSelectedArtifact}
            />
          </ArtifactsProvider>
        </FakeContextProvider>
      </VirtuosoMockContext.Provider>,
    );

    // Wait for the tree to render.
    await screen.findByText('Result artifacts');
    const artifactNode = screen.getByText('artifact1.log');
    expect(artifactNode).toBeInTheDocument();

    // The component should select the summary or first leaf node automatically on render.
    expect(updateSelectedArtifact).toHaveBeenCalled();

    // Click another artifact and confirm the callback is fired with the correct data.
    await user.click(artifactNode);
    expect(updateSelectedArtifact).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'artifact1.log',
        source: 'result',
      }),
    );
  });

  it('should filter the artifact tree when a user types in the search box', async () => {
    jest.useFakeTimers();
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime });
    const updateSelectedArtifact = jest.fn();

    render(
      <VirtuosoMockContext.Provider
        value={{ viewportHeight: 300, itemHeight: 30 }}
      >
        <FakeContextProvider>
          <ArtifactsProvider results={MOCK_RESULTS}>
            <ArtifactTreeView
              resultArtifacts={resultArtifacts}
              invArtifacts={invArtifacts}
              artifactsLoading={false}
              updateSelectedArtifact={updateSelectedArtifact}
            />
          </ArtifactsProvider>
        </FakeContextProvider>
      </VirtuosoMockContext.Provider>,
    );

    const searchBox = screen.getByPlaceholderText('Search for artifact');
    await user.type(searchBox, 'log');

    await act(async () => {
      jest.runAllTimers();
    });

    // We expect 5 items: Summary + "Result artifacts" + "Invocation artifacts" + the two log files.
    const treeItems = screen.getAllByRole('treeitem');
    expect(treeItems).toHaveLength(5);

    expect(screen.queryByText('artifact2.png')).not.toBeInTheDocument();
    expect(screen.queryByText('invArtifact2.jpg')).not.toBeInTheDocument();

    jest.useRealTimers();
  });
});
