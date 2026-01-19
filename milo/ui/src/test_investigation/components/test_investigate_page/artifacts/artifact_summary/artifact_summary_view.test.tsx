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

import { useQuery } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { useFetchArtifactContentQuery } from '@/test_investigation/hooks/queries';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ArtifactSummaryView } from './artifact_summary_view';

jest.mock('@/common/hooks/prpc_clients', () => ({
  useResultDbClient: jest.fn(),
}));

jest.mock('@tanstack/react-query', () => ({
  ...jest.requireActual('@tanstack/react-query'),
  useQuery: jest.fn(),
}));

jest.mock('@/test_investigation/context', () => ({
  useIsLegacyInvocation: jest.fn().mockReturnValue(false),
  useTestVariant: jest.fn().mockReturnValue({ variant: { def: {} } }),
}));

jest.mock('@/test_investigation/hooks/queries', () => ({
  useFetchArtifactContentQuery: jest.fn(),
}));

describe('<ArtifactSummaryView />', () => {
  const mockTestResult = TestResult.fromPartial({
    name: 'invocations/inv/tests/test-id/results/result-id',
    testId: 'test-id',
    resultId: 'result-id',
  });

  beforeEach(() => {
    (useResultDbClient as jest.Mock).mockReturnValue({
      GetArtifact: {
        query: jest.fn().mockImplementation((req) => ({
          queryKey: ['resultdb', 'GetArtifact', req],
          ...req,
        })),
      },
    });

    (useQuery as jest.Mock).mockReturnValue({
      data: undefined,
      isLoading: false,
    });

    (useFetchArtifactContentQuery as jest.Mock).mockReturnValue({
      data: { data: 'some diff content' },
      isLoading: false,
    });
  });

  it('should query for text diff artifact', () => {
    render(
      <FakeContextProvider>
        <ArtifactSummaryView
          currentResult={mockTestResult}
          selectedAttemptIndex={0}
        />
      </FakeContextProvider>,
    );

    expect(useQuery).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'invocations/inv/tests/test-id/results/result-id/artifacts/text_diff',
      }),
    );
  });

  it('should display text diff artifact when available', () => {
    const mockTextDiffArtifact = Artifact.fromPartial({
      name: 'invocations/inv/tests/test-id/results/result-id/artifacts/text_diff',
      artifactId: 'text_diff',
    });

    (useQuery as jest.Mock).mockImplementation((options) => {
      const queryKey = options?.queryKey;
      if (
        Array.isArray(queryKey) &&
        queryKey.length > 2 &&
        queryKey[2]?.name?.endsWith('text_diff')
      ) {
        return {
          data: mockTextDiffArtifact,
          isLoading: false,
        };
      }
      return {
        data: undefined,
        isLoading: false,
      };
    });

    render(
      <FakeContextProvider>
        <ArtifactSummaryView
          currentResult={mockTestResult}
          selectedAttemptIndex={0}
        />
      </FakeContextProvider>,
    );

    expect(screen.getByText('Text Diff')).toBeInTheDocument();
  });
});
