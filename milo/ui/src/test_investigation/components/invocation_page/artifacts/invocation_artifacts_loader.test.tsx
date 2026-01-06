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

import { useInfiniteQuery } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { useArtifactFilters } from '@/test_investigation/components/common/artifacts/tree/context/context';
import { useInvocation } from '@/test_investigation/context';

import { InvocationArtifactsLoader } from './invocation_artifacts_loader';

jest.mock('@tanstack/react-query', () => ({
  useInfiniteQuery: jest.fn(),
  InfiniteData: jest.requireActual('@tanstack/react-query').InfiniteData,
}));

jest.mock('@/common/hooks/prpc_clients');
jest.mock('@/test_investigation/context');
jest.mock(
  '@/test_investigation/components/common/artifacts/tree/context/context',
);
jest.mock(
  '@/test_investigation/components/common/artifacts/context/provider',
  () => ({
    ArtifactsProvider: ({
      nodes,
      children,
    }: {
      nodes: unknown[];
      children: React.ReactNode;
    }) => (
      <div data-testid="artifacts-provider" data-nodes-count={nodes.length}>
        {children}
      </div>
    ),
  }),
);

const mockUseResultDbClient = useResultDbClient as jest.Mock;
const mockUseInvocation = useInvocation as jest.Mock;
const mockUseArtifactFilters = useArtifactFilters as jest.Mock;
const mockUseInfiniteQuery = useInfiniteQuery as jest.Mock;

describe('InvocationArtifactsLoader', () => {
  beforeEach(() => {
    mockUseResultDbClient.mockReturnValue({
      ListArtifacts: {
        queryPaged: jest.fn(() => ({})),
      },
    });

    mockUseInvocation.mockReturnValue({
      name: 'invocations/inv-123',
    });

    mockUseArtifactFilters.mockReturnValue({
      debouncedSearchTerm: '',
      artifactTypes: [],
      hideEmptyFolders: false,
      setAvailableArtifactTypes: jest.fn(),
    });
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it('renders loading state initially', () => {
    mockUseInfiniteQuery.mockReturnValue({
      data: undefined,
      isPending: true,
      hasNextPage: false,
      fetchNextPage: jest.fn(),
    });

    render(
      <InvocationArtifactsLoader>
        <div>Child Content</div>
      </InvocationArtifactsLoader>,
    );

    expect(screen.getByText('Loading artifacts...')).toBeInTheDocument();
  });

  it('renders children and provides artifacts when loaded', () => {
    const mockArtifacts: Artifact[] = [
      Artifact.fromPartial({
        name: 'invocations/inv-123/artifacts/foo.txt',
        artifactId: 'foo.txt',
      }),
      Artifact.fromPartial({
        name: 'invocations/inv-123/artifacts/dir/bar.txt',
        artifactId: 'dir/bar.txt',
      }),
    ];

    mockUseInfiniteQuery.mockReturnValue({
      data: mockArtifacts,
      isPending: false,
      hasNextPage: false,
      fetchNextPage: jest.fn(),
    });

    render(
      <InvocationArtifactsLoader>
        <div>Child Content</div>
      </InvocationArtifactsLoader>,
    );

    expect(screen.getByText('Child Content')).toBeInTheDocument();

    const provider = screen.getByTestId('artifacts-provider');
    expect(Number(provider.getAttribute('data-nodes-count'))).toBeGreaterThan(
      0,
    );
  });

  it('uses correct parent for RootInvocation', () => {
    // Mock as RootInvocation
    mockUseInvocation.mockReturnValue({
      name: 'invocations/inv-root',
      rootInvocationId: 'inv-root',
    });

    mockUseInfiniteQuery.mockReturnValue({
      data: undefined,
      isPending: true,
      hasNextPage: false,
    });

    render(
      <InvocationArtifactsLoader>
        <div>Child Content</div>
      </InvocationArtifactsLoader>,
    );

    // Verify the query was called with the correct parent
    expect(
      mockUseResultDbClient().ListArtifacts.queryPaged,
    ).toHaveBeenCalledWith(
      expect.objectContaining({
        parent: 'invocations/inv-root/workUnits/root',
      }),
    );
  });
});
