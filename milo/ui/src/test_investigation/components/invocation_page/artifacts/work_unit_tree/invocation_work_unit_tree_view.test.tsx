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
import { useInfiniteQuery, useQueries, useQuery } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { VirtuosoMockContext } from 'react-virtuoso';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { RootInvocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/root_invocation.pb';
import {
  BatchGetWorkUnitsRequest,
  WorkUnit,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/work_unit.pb';
import { ArtifactsProvider } from '@/test_investigation/components/common/artifacts/context';
import {
  ArtifactFiltersContext,
  ArtifactFiltersContextValue,
} from '@/test_investigation/components/common/artifacts/tree/context/context';
import { ArtifactTreeNodeData } from '@/test_investigation/components/common/artifacts/types';
import { useInvocation } from '@/test_investigation/context/context';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { InvocationWorkUnitTreeView } from './invocation_work_unit_tree_view';

// Mock dependencies
jest.mock('@/common/hooks/prpc_clients', () => ({
  useResultDbClient: jest.fn(),
}));

jest.mock('@tanstack/react-query', () => ({
  ...jest.requireActual('@tanstack/react-query'),
  useQuery: jest.fn(),
  useQueries: jest.fn(),
  useInfiniteQuery: jest.fn(),
}));

jest.mock('@/generic_libs/hooks/synced_search_params', () => ({
  ...jest.requireActual('@/generic_libs/hooks/synced_search_params'),
  useSyncedSearchParams: jest.fn(),
}));

jest.mock('@/test_investigation/context/context', () => ({
  useInvocation: jest.fn(),
}));

jest.mock(
  '../../../common/artifacts/tree/artifact_tree_node/artifact_tree_node',
  () => ({
    ArtifactTreeNode: ({ row }: { row: { name: string } }) => (
      <div data-testid="artifact-tree-node">{row.name}</div>
    ),
  }),
);

// Define types for mocks
interface MockQueryOptions {
  queryKey?: readonly unknown[];
}

interface MockBatchGetWorkUnitsRequest {
  names: string[];
}

interface MockUseQueriesEntry {
  queryKey?: readonly unknown[];
}

interface MockUseQueriesOptions {
  queries?: readonly MockUseQueriesEntry[];
}

describe('<InvocationWorkUnitTreeView />', () => {
  const createMockArtifactFiltersContext = (
    overrides: Partial<ArtifactFiltersContextValue> = {},
  ): ArtifactFiltersContextValue => ({
    searchTerm: '',
    setSearchTerm: jest.fn(),
    debouncedSearchTerm: '',
    artifactTypes: [],
    setArtifactTypes: jest.fn(),
    crashTypes: [],
    setCrashTypes: jest.fn(),
    showCriticalCrashes: false,
    setShowCriticalCrashes: jest.fn(),
    hideAutomationFiles: false,
    setHideAutomationFiles: jest.fn(),
    hideEmptyFolders: true,
    setHideEmptyFolders: jest.fn(),
    showOnlyFoldersWithError: false,
    setShowOnlyFoldersWithError: jest.fn(),
    onClearFilters: jest.fn(),
    availableArtifactTypes: [],
    setAvailableArtifactTypes: jest.fn(),
    isFilterPanelOpen: false,
    setIsFilterPanelOpen: jest.fn(),
    ...overrides,
  });

  const mockRootInvocationId = 'inv-id';

  beforeEach(() => {
    (useResultDbClient as jest.Mock).mockReturnValue({
      GetWorkUnit: {
        query: jest.fn((req) => ({ queryKey: ['GetWorkUnit', req] })),
      },
      BatchGetWorkUnits: {
        query: jest.fn((req) => ({ queryKey: ['BatchGetWorkUnits', req] })),
      },
      ListArtifacts: {
        query: jest.fn((req) => ({ queryKey: ['ListArtifacts', req] })),
      },
      QueryArtifacts: {
        queryPaged: jest.fn((req) => ({
          queryKey: ['QueryArtifacts', req],
          initialPageParam: '',
        })),
      },
    });

    // Default generic mock for queries prevents excessive errors
    (useQuery as jest.Mock).mockImplementation(() => ({
      data: null,
      isLoading: false,
    }));
    (useQueries as jest.Mock).mockImplementation(() => []);
    (useInfiniteQuery as jest.Mock).mockReturnValue({
      data: { pages: [] },
      isLoading: false,
      fetchNextPage: jest.fn(),
      hasNextPage: false,
      isFetchingNextPage: false,
    });

    // Default mock for useSyncedSearchParams: no_empty='false' (disabled by default for tests)
    const mockSearchParams = new Map<string, string>([['no_empty', 'false']]);
    (useSyncedSearchParams as jest.Mock).mockReturnValue([
      { get: (key: string) => mockSearchParams.get(key) || null },
      jest.fn(),
    ]);

    (useInvocation as jest.Mock).mockReturnValue({
      name: 'rootInvocations/inv-id',
      rootInvocationId: 'inv-id',
    });
  });

  it('should fetch root work unit and render it', async () => {
    const mockRootWorkUnit = WorkUnit.fromPartial({
      name: 'rootInvocations/inv-id/workUnits/root',
      workUnitId: 'root',
      childWorkUnits: ['rootInvocations/inv-id/workUnits/child1'],
    });

    (useQuery as jest.Mock).mockImplementation((options: MockQueryOptions) => {
      const queryKey = options?.queryKey || [];
      if (queryKey.includes('GetWorkUnit')) {
        return { data: mockRootWorkUnit, isLoading: false };
      }
      return { data: null, isLoading: false };
    });

    (useQueries as jest.Mock).mockImplementation(
      (props: MockUseQueriesOptions) => {
        const queries = props?.queries || [];
        return queries.map((q) => {
          const request = q.queryKey?.[1] as BatchGetWorkUnitsRequest;
          const names = request?.names || [];

          const returnedWorkUnits: WorkUnit[] = [];

          if (names.some((n: string) => n.endsWith('/child1'))) {
            returnedWorkUnits.push(
              WorkUnit.fromPartial({
                name: 'rootInvocations/inv-id/workUnits/child1',
                workUnitId: 'child1',
                parent: 'rootInvocations/inv-id/workUnits/root',
                childWorkUnits: ['rootInvocations/inv-id/workUnits/child2'],
              }),
            );
          }

          if (names.some((n: string) => n.endsWith('/child2'))) {
            returnedWorkUnits.push(
              WorkUnit.fromPartial({
                name: 'rootInvocations/inv-id/workUnits/child2',
                workUnitId: 'child2',
                parent: 'rootInvocations/inv-id/workUnits/child1',
              }),
            );
          }

          return {
            data: {
              workUnits: returnedWorkUnits,
            },
            isLoading: false,
          };
        });
      },
    );

    const mockContextValue = createMockArtifactFiltersContext({
      hideEmptyFolders: false,
    });
    const mockInvocation = RootInvocation.fromPartial({
      name: 'rootInvocations/inv-id',
      rootInvocationId: 'inv-id',
    });
    const mockNodes: ArtifactTreeNodeData[] = [];
    const mockVirtuosoContext = { viewportHeight: 300, itemHeight: 30 };

    render(
      <FakeContextProvider>
        <VirtuosoMockContext.Provider value={mockVirtuosoContext}>
          <ArtifactFiltersContext.Provider value={mockContextValue}>
            <ArtifactsProvider nodes={mockNodes} invocation={mockInvocation}>
              <InvocationWorkUnitTreeView
                rootInvocationId={mockRootInvocationId}
              />
            </ArtifactsProvider>
          </ArtifactFiltersContext.Provider>
        </VirtuosoMockContext.Provider>
      </FakeContextProvider>,
    );

    expect(await screen.findByText('root')).toBeInTheDocument();
    expect(await screen.findByText('child1')).toBeInTheDocument();
    expect(await screen.findByText('child2')).toBeInTheDocument();
  });

  it('should render mapped work unit label', async () => {
    const mockRootWorkUnit = WorkUnit.fromPartial({
      name: 'rootInvocations/inv-id/workUnits/root',
      workUnitId: 'root',
      childWorkUnits: ['rootInvocations/inv-id/workUnits/wu2'],
    });

    (useQuery as jest.Mock).mockImplementation((options: MockQueryOptions) => {
      const queryKey = options?.queryKey || [];
      if (queryKey.includes('GetWorkUnit')) {
        return { data: mockRootWorkUnit, isLoading: false };
      }
      return { data: null, isLoading: false };
    });

    (useQueries as jest.Mock).mockImplementation(
      (props: MockUseQueriesOptions) => {
        const queries = props?.queries || [];
        return queries.map((q) => {
          const request = q.queryKey?.[1] as BatchGetWorkUnitsRequest;
          const names = request?.names || [];
          if (names.some((n: string) => n.endsWith('/wu2'))) {
            return {
              data: {
                workUnits: [
                  WorkUnit.fromPartial({
                    name: 'rootInvocations/inv-id/workUnits/wu2',
                    workUnitId: 'wu2',
                    kind: 'TFC_COMMAND',
                  }),
                ],
              },
              isLoading: false,
            };
          }
          return { data: { workUnits: [] }, isLoading: false };
        });
      },
    );

    const mockContextValue = createMockArtifactFiltersContext({
      hideEmptyFolders: false,
    });
    const mockInvocation = RootInvocation.fromPartial({
      name: 'rootInvocations/inv-id',
      rootInvocationId: 'inv-id',
    });
    const mockNodes: ArtifactTreeNodeData[] = [];
    const mockVirtuosoContext = { viewportHeight: 300, itemHeight: 30 };

    render(
      <FakeContextProvider>
        <VirtuosoMockContext.Provider value={mockVirtuosoContext}>
          <ArtifactFiltersContext.Provider value={mockContextValue}>
            <ArtifactsProvider nodes={mockNodes} invocation={mockInvocation}>
              <InvocationWorkUnitTreeView
                rootInvocationId={mockRootInvocationId}
              />
            </ArtifactsProvider>
          </ArtifactFiltersContext.Provider>
        </VirtuosoMockContext.Provider>
      </FakeContextProvider>,
    );

    expect(await screen.findByText('TFC COMMAND')).toBeInTheDocument();
  });

  it('should prune empty work units when hideEmptyWorkUnits is true', async () => {
    // Setup: Root -> Child1 (empty)
    const rootWU = WorkUnit.fromPartial({
      name: 'rootInvocations/inv-id/workUnits/root',
      workUnitId: 'root',
      childWorkUnits: ['rootInvocations/inv-id/workUnits/child1'],
    });
    const child1 = WorkUnit.fromPartial({
      name: 'rootInvocations/inv-id/workUnits/child1',
      workUnitId: 'child1',
    });

    (useQuery as jest.Mock).mockImplementation((options: MockQueryOptions) => {
      const queryKey = options?.queryKey || [];
      if (queryKey.includes('GetWorkUnit')) {
        return { data: rootWU, isLoading: false };
      }
      return { data: null, isLoading: false };
    });

    (useQueries as jest.Mock).mockImplementation(
      (props: MockUseQueriesOptions) => {
        const queries = props?.queries || [];
        return queries.map((query) => {
          const queryKey = query?.queryKey || [];
          if (queryKey.includes('BatchGetWorkUnits')) {
            const request = queryKey.find(
              (item): item is MockBatchGetWorkUnitsRequest =>
                typeof item === 'object' &&
                item !== null &&
                'names' in item &&
                Array.isArray(
                  (item as unknown as MockBatchGetWorkUnitsRequest).names,
                ),
            );
            if (request && request.names.includes(child1.name)) {
              return { data: { workUnits: [child1] }, isLoading: false };
            }
          }
          return { data: null, isLoading: false };
        });
      },
    );

    const mockSearchParams = new Map<string, string>([['no_empty', 'true']]);
    (useSyncedSearchParams as jest.Mock).mockReturnValue([
      { get: (key: string) => mockSearchParams.get(key) || null },
      jest.fn(),
    ]);

    const mockContextValue = createMockArtifactFiltersContext({
      hideEmptyFolders: true,
    });
    const mockInvocation = RootInvocation.fromPartial({
      name: 'rootInvocations/inv-id',
      rootInvocationId: 'inv-id',
    });
    const mockNodes: ArtifactTreeNodeData[] = [];
    const mockVirtuosoContext = { viewportHeight: 300, itemHeight: 30 };

    render(
      <FakeContextProvider>
        <VirtuosoMockContext.Provider value={mockVirtuosoContext}>
          <ArtifactFiltersContext.Provider value={mockContextValue}>
            <ArtifactsProvider nodes={mockNodes} invocation={mockInvocation}>
              <InvocationWorkUnitTreeView
                rootInvocationId={mockRootInvocationId}
              />
            </ArtifactsProvider>
          </ArtifactFiltersContext.Provider>
        </VirtuosoMockContext.Provider>
      </FakeContextProvider>,
    );

    // The component renders "Summary" always.
    expect(await screen.findByText('Summary')).toBeInTheDocument();
    expect(screen.queryByText('No work units found.')).not.toBeInTheDocument();
    expect(screen.queryByText('root')).not.toBeInTheDocument();
  });
});
