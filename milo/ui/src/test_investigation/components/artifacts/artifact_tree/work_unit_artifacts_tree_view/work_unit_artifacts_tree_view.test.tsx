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

import { useQueries, useQuery } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { VirtuosoMockContext } from 'react-virtuoso';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { WorkUnit } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/work_unit.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { useArtifactsContext } from '../../context';
import { ArtifactFilterProvider } from '../context';

import { WorkUnitArtifactsTreeView } from './work_unit_artifacts_tree_view';

jest.mock('../../context', () => ({
  useArtifactsContext: jest.fn(),
  ArtifactsProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}));

jest.mock('@/common/hooks/prpc_clients', () => ({
  useResultDbClient: jest.fn(),
}));

jest.mock('@tanstack/react-query', () => ({
  ...jest.requireActual('@tanstack/react-query'),
  useQuery: jest.fn(),
  useQueries: jest.fn(),
}));

jest.mock('@/test_investigation/context', () => ({
  useInvocation: jest.fn().mockReturnValue({ name: 'invocations/inv' }),
}));

describe('<WorkUnitArtifactsTreeView />', () => {
  const mockWorkUnits: WorkUnit[] = [
    WorkUnit.fromPartial({
      name: 'invocations/inv/workUnits/wu0',
      workUnitId: 'wu0',
    }),
  ];

  const mockArtifacts: Artifact[] = [
    Artifact.fromPartial({
      name: 'invocations/inv/workUnits/wu0/artifacts/log.txt',
      artifactId: 'log.txt',
    }),
  ];

  const mockTargetArtifacts: Artifact[] = [
    Artifact.fromPartial({
      name: 'invocations/inv/workUnits/wu1/artifacts/result.txt',
      artifactId: 'result.txt',
      hasLines: true,
    }),
  ];

  beforeEach(() => {
    (useArtifactsContext as jest.Mock).mockReturnValue({
      selectedArtifact: null,
      setSelectedArtifact: jest.fn(),
      clusteredFailures: [],
      hasRenderableResults: false,
    });

    (useResultDbClient as jest.Mock).mockReturnValue({
      QueryWorkUnits: {
        query: jest.fn().mockReturnValue({}),
      },
      ListArtifacts: {
        query: jest.fn().mockReturnValue({}),
      },
      GetWorkUnit: {
        query: jest.fn().mockReturnValue({}),
      },
    });

    (useQuery as jest.Mock).mockImplementation((query) => {
      if (query === undefined) {
        return { data: undefined, isLoading: false };
      }
      // Mock GetWorkUnit response
      if (query.name && query.name.includes('GetWorkUnit')) {
        return {
          data: WorkUnit.fromPartial({
            name: 'invocations/inv/workUnits/wu1',
            workUnitId: 'wu1',
            kind: 'Test Result', // Mocked kind
          }),
          isLoading: false,
        };
      }
      return {
        data: { workUnits: mockWorkUnits },
        isLoading: false,
      };
    });

    (useQueries as jest.Mock).mockReturnValue([
      {
        data: { artifacts: mockArtifacts },
        isLoading: false,
      },
      {
        data: { artifacts: mockTargetArtifacts },
        isLoading: false,
      },
    ]);
  });

  it('should render work units and artifacts', async () => {
    // Mock the GetWorkUnit query key to match what useQuery expects for the mock implementation above
    const mockGetWorkUnitQuery = {
      name: 'GetWorkUnit',
    };
    (useResultDbClient as jest.Mock).mockReturnValue({
      QueryWorkUnits: {
        query: jest
          .fn()
          .mockImplementation((req) => ({ ...req, _type: 'QueryWorkUnits' })),
      },
      ListArtifacts: {
        query: jest
          .fn()
          .mockImplementation((req) => ({ ...req, _type: 'ListArtifacts' })),
      },
      GetWorkUnit: {
        query: jest.fn().mockReturnValue(mockGetWorkUnitQuery),
      },
    });

    (useQueries as jest.Mock).mockReturnValue([
      {
        data: { artifacts: mockArtifacts },
        isLoading: false,
      },
      {
        data: { artifacts: mockTargetArtifacts },
        isLoading: false,
      },
    ]);

    // Mock useQuery for ListArtifacts for Test Result artifacts
    (useQuery as jest.Mock).mockImplementation((query) => {
      if (query === undefined) {
        return { data: undefined, isLoading: false };
      }
      if (query.name && query.name.includes('GetWorkUnit')) {
        return {
          data: WorkUnit.fromPartial({
            name: 'invocations/inv/workUnits/wu1',
            workUnitId: 'wu1',
            kind: 'Test Result',
          }),
          isLoading: false,
        };
      }
      // Mock ListArtifacts for currentResult (Test Result artifacts)
      if (
        query.parent &&
        query.parent.includes('invocations/inv/tests/test-id/results/result-id')
      ) {
        return {
          data: {
            artifacts: [
              Artifact.fromPartial({
                name: 'invocations/inv/tests/test-id/results/result-id/artifacts/test_result_artifact.txt',
                artifactId: 'test_result_artifact.txt',
                hasLines: true,
                artifactType: 'text',
              }),
            ],
          },
          isLoading: false,
        };
      }

      // Default fallback
      return {
        data: { workUnits: mockWorkUnits },
        isLoading: false,
      };
    });

    (useArtifactsContext as jest.Mock).mockReturnValue({
      selectedArtifact: null,
      setSelectedArtifact: jest.fn(),
      clusteredFailures: [],
      hasRenderableResults: false,
      currentResult: {
        name: 'invocations/inv/tests/test-id/results/result-id',
      },
    });

    render(
      <VirtuosoMockContext.Provider
        value={{ viewportHeight: 300, itemHeight: 30 }}
      >
        <FakeContextProvider>
          <ArtifactFilterProvider>
            <WorkUnitArtifactsTreeView
              rootInvocationId="inv"
              workUnitId="wu1"
            />
          </ArtifactFilterProvider>
        </FakeContextProvider>
      </VirtuosoMockContext.Provider>,
    );

    await screen.findByText('wu0');
    expect(screen.getByText('log.txt')).toBeInTheDocument();

    // Wait for Test Result artifacts to load
    expect(await screen.findAllByText('Test Result')).toHaveLength(2);
    expect(screen.getByText('test_result_artifact.txt')).toBeInTheDocument();
    expect(screen.getByText('result.txt')).toBeInTheDocument();
  });
});
