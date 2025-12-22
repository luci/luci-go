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
import userEvent from '@testing-library/user-event';
import { VirtuosoMockContext } from 'react-virtuoso';

import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';

import { ArtifactsProvider } from '../../context/provider';
import { ArtifactTreeNodeData } from '../../types';
import { useArtifactFilters } from '../context/context';

import { ArtifactTreeView } from './artifact_tree_view';

jest.mock('../context/context', () => ({
  useArtifactFilters: jest.fn(),
}));

// Mock window.open
window.open = jest.fn();

describe('<ArtifactTreeView />', () => {
  const nodes: ArtifactTreeNodeData[] = [
    {
      id: '1',
      name: 'artifact1.log',
      children: [],
      artifact: Artifact.fromPartial({
        name: 'invocations/inv/artifacts/artifact1.log',
        artifactId: 'artifact1.log',
        hasLines: true,
      }),
      source: 'result',
      viewingSupported: true,
    },
    {
      id: '2',
      name: 'folder',
      children: [
        {
          id: '3',
          name: 'artifact2.txt',
          children: [],
          artifact: Artifact.fromPartial({
            name: 'invocations/inv/artifacts/folder/artifact2.txt',
            artifactId: 'folder/artifact2.txt',
            hasLines: true,
          }),
          source: 'result',
          viewingSupported: true,
        },
      ],
      source: 'result',
    },
  ];

  const mockInvocation: AnyInvocation = {
    name: 'invocations/inv',
  } as AnyInvocation;

  beforeEach(() => {
    (useArtifactFilters as jest.Mock).mockReturnValue({
      debouncedSearchTerm: '',
    });
  });

  it('should render nodes correctly', async () => {
    render(
      <VirtuosoMockContext.Provider
        value={{ viewportHeight: 300, itemHeight: 30 }}
      >
        <ArtifactsProvider nodes={nodes} invocation={mockInvocation}>
          <ArtifactTreeView />
        </ArtifactsProvider>
      </VirtuosoMockContext.Provider>,
    );

    await screen.findByText('artifact1.log');
    expect(screen.getByText('folder')).toBeInTheDocument();
  });

  it('should handle selection', async () => {
    const onSelect = jest.fn();
    const user = userEvent.setup();

    render(
      <VirtuosoMockContext.Provider
        value={{ viewportHeight: 300, itemHeight: 30 }}
      >
        <ArtifactsProvider
          nodes={nodes}
          invocation={mockInvocation}
          onSelect={onSelect}
        >
          <ArtifactTreeView />
        </ArtifactsProvider>
      </VirtuosoMockContext.Provider>,
    );

    await screen.findByText('artifact1.log');
    await user.click(screen.getByText('artifact1.log'));

    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: '1' }));
  });

  it('should highlight search terms', async () => {
    (useArtifactFilters as jest.Mock).mockReturnValue({
      debouncedSearchTerm: 'art',
    });

    render(
      <VirtuosoMockContext.Provider
        value={{ viewportHeight: 300, itemHeight: 30 }}
      >
        <ArtifactsProvider nodes={nodes} invocation={mockInvocation}>
          <ArtifactTreeView />
        </ArtifactsProvider>
      </VirtuosoMockContext.Provider>,
    );

    // Virtuoso acts async often, but here we mock context.
    // The highlighting is done in ArtifactTreeNode logic which is used by ArtifactTreeView.
    // Verify 'art' is highlighted (inside strong tag)
    // Note: Use findAllByText because multiple artifacts might match "art"
    const strongs = await screen.findAllByText('art', { selector: 'strong' });
    expect(strongs.length).toBeGreaterThan(0);

    // Verify the full text is present in the parent container
    // Verify the full text is present in the parent container
    const treeItems = await screen.findAllByRole('treeitem');
    const match = treeItems.find((item) =>
      item.textContent?.includes('artifact1.log'),
    );
    expect(match).toBeInTheDocument();
  });
});
