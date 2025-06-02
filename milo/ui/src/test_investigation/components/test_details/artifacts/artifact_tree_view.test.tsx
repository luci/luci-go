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
import { VirtuosoMockContext } from 'react-virtuoso';

import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ArtifactTreeView } from './artifact_tree_view';

describe('<ArtifactTreeView />', () => {
  const resultArtifacts: readonly Artifact[] = [
    Artifact.fromPartial({
      artifactId: 'artifact1',
      name: 'artifacts/artifact1',
    }),
    Artifact.fromPartial({
      artifactId: 'artifact2',
      name: 'artifacts/artifact2',
    }),
  ];

  const invArtifacts: readonly Artifact[] = [
    Artifact.fromPartial({
      artifactId: 'invArtifact1',
      name: 'invArtifacts/invArtifact1',
    }),
    Artifact.fromPartial({
      artifactId: 'invArtifact2',
      name: 'invArtifacts/invArtifact2',
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
    expect(screen.getByText('artifact1')).toBeInTheDocument();
    expect(screen.getByText('artifact2')).toBeInTheDocument();
    expect(screen.getByText('invArtifact1')).toBeInTheDocument();
    expect(screen.getByText('invArtifact2')).toBeInTheDocument();
  });
});
