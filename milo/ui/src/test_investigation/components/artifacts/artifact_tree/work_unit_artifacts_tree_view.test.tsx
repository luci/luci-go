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

import { useArtifactsContext } from '../context';

import { WorkUnitArtifactsTreeView } from './work_unit_artifacts_tree_view';

jest.mock('../context', () => ({
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
      name: 'invocations/inv/workUnits/wu1',
      workUnitId: 'wu1',
    }),
  ];

  const mockArtifacts: Artifact[] = [
    Artifact.fromPartial({
      name: 'invocations/inv/workUnits/wu1/artifacts/log.txt',
      artifactId: 'log.txt',
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
    });

    (useQuery as jest.Mock).mockReturnValue({
      data: { workUnits: mockWorkUnits },
      isLoading: false,
    });

    (useQueries as jest.Mock).mockReturnValue([
      {
        data: { artifacts: mockArtifacts },
        isLoading: false,
      },
    ]);
  });

  it('should render work units and artifacts', async () => {
    render(
      <VirtuosoMockContext.Provider
        value={{ viewportHeight: 300, itemHeight: 30 }}
      >
        <FakeContextProvider>
          <WorkUnitArtifactsTreeView rootInvocationId="inv" workUnitId="wu1" />
        </FakeContextProvider>
      </VirtuosoMockContext.Provider>,
    );

    expect(await screen.findByText('wu1')).toBeInTheDocument();
    expect(screen.getByText('log.txt')).toBeInTheDocument();
  });
});
