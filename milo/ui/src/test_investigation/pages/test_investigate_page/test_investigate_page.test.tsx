// Copyright 2026 The LUCI Authors.
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

import { render, screen, waitFor } from '@testing-library/react';
import React from 'react';
import * as ReactRouter from 'react-router';

import {
  useBatchedClustersClient,
  useResultDbClient,
  useTestHistoryClient,
  useTestVariantBranchesClient,
} from '@/common/hooks/prpc_clients';
import { OutputTestVerdict } from '@/common/types/verdict';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestVerdict_StatusOverride } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';
import { TestVerdict_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';
import { useInvocationQuery } from '@/test_investigation/hooks/queries';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TestInvestigatePage } from './test_investigate_page';

jest.mock('@/common/hooks/prpc_clients');
jest.mock('@/test_investigation/hooks/queries');

jest.mock('@/generic_libs/components/google_analytics', () => ({
  __esModule: true,
  useGoogleAnalytics: () => ({ trackEvent: jest.fn() }),
  TrackLeafRoutePageView: () => null,
}));

// Mock Sticky components to avoid layout issues in test environment
jest.mock('@/generic_libs/components/queued_sticky', () => ({
  Sticky: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  StickyOffset: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

jest.mock('@/common/tools/logging', () => ({
  logging: { error: jest.fn(), warn: jest.fn() },
}));
jest.mock('@/fleet/utils/survey', () => ({
  requestSurvey: jest.fn(),
}));

// Mock child components to simplify the test
jest.mock('@/test_investigation/components/redirect_back_banner', () => ({
  RedirectBackBanner: () => <div>RedirectBackBanner</div>,
}));
jest.mock(
  '@/test_investigation/components/redirect_back_banner/redirect_ati_banner',
  () => ({
    RedirectATIBanner: () => <div>RedirectATIBanner</div>,
  }),
);
jest.mock('@/test_investigation/components/test_info', () => ({
  TestInfo: () => <div>TestInfo</div>,
}));
jest.mock('@/test_investigation/components/test_info/test_info_header', () => ({
  TestInfoHeader: () => <div>TestInfoHeader</div>,
}));
jest.mock(
  '@/test_investigation/components/test_investigate_page/artifacts/artifacts_section',
  () => ({
    ArtifactsSection: () => <div>ArtifactsSection</div>,
  }),
);
jest.mock(
  '@/test_investigation/components/test_info/collapsed_test_info_header',
  () => ({
    CollapsedTestInfoHeader: () => <div>Collapsedheader</div>,
  }),
);
jest.mock('@/test_investigation/components/test_navigation_drawer', () => ({
  TestNavigationDrawer: () => <div>TestNavigationDrawer</div>,
}));

describe('TestInvestigatePage', () => {
  const mockBatchGetTestVariants = jest.fn();
  const mockQueryRecentPasses = jest.fn();
  const mockClusterQuery = jest.fn();
  const mockBranchesQueryPaged = jest.fn();
  const mockQueryTestVariants = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();

    (useResultDbClient as jest.Mock).mockReturnValue({
      BatchGetTestVariants: { query: mockBatchGetTestVariants },
      QueryTestVariants: { query: mockQueryTestVariants },
    });
    (useTestHistoryClient as jest.Mock).mockReturnValue({
      QueryRecentPasses: { query: mockQueryRecentPasses },
    });
    (useBatchedClustersClient as jest.Mock).mockReturnValue({
      Cluster: { query: mockClusterQuery },
    });
    (useTestVariantBranchesClient as jest.Mock).mockReturnValue({
      Query: { queryPaged: mockBranchesQueryPaged },
    });

    // Default mock implementation for useQuery
    mockBatchGetTestVariants.mockImplementation((req) => ({
      queryKey: ['batchGetTestVariants', req],
      queryFn: jest.fn().mockResolvedValue({
        testVariants: [
          {
            testId: 'test-id',
            variantHash: 'hash',
            statusV2: TestVerdict_Status.PASSED,
            statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
          } as OutputTestVerdict,
        ],
      }),
    }));

    mockQueryRecentPasses.mockImplementation((req) => ({
      queryKey: ['queryRecentPasses', req],
      queryFn: jest.fn().mockResolvedValue({ passingResults: [] }),
    }));

    mockClusterQuery.mockImplementation((req) => ({
      queryKey: ['clusterQuery', req],
      queryFn: jest.fn().mockResolvedValue({ clusteredTestResults: [] }),
    }));

    mockBranchesQueryPaged.mockImplementation((req) => ({
      queryKey: ['branchesQueryPaged', req],
      queryFn: jest.fn().mockResolvedValue({ pages: [] }),
    }));

    mockQueryTestVariants.mockImplementation((req) => ({
      queryKey: ['queryTestVariants', req],
      queryFn: jest.fn().mockResolvedValue({ testVariants: [] }),
    }));
  });

  const renderComponent = (isLegacy = false) => {
    const invocationName = isLegacy ? 'inv-id' : 'inv-id';
    (useInvocationQuery as jest.Mock).mockReturnValue({
      invocation: {
        data: {
          name: `invocations/${invocationName}`,
          realm: 'project:realm',
        } as Invocation,
        isLegacyInvocation: isLegacy,
      },
      isLoading: false,
      errors: [],
    });

    render(
      <FakeContextProvider>
        <TestInvestigatePage />
      </FakeContextProvider>,
    );
  };

  it('should use "invocation" field for legacy invocations', async () => {
    // Mock useParams behavior
    jest.spyOn(ReactRouter, 'useParams').mockReturnValue({
      invocationId: 'inv-id',
      module: 'module',
      scheme: 'scheme',
      variant: 'hash',
      case: 'test-id',
    });

    renderComponent(true);

    await waitFor(() => {
      expect(screen.getByText('TestInfo')).toBeInTheDocument();
    });

    expect(mockBatchGetTestVariants).toHaveBeenCalledWith(
      expect.objectContaining({
        invocation: 'invocations/inv-id',
        // In some proto implementations, unused string fields default to empty string
        parent: '',
      }),
    );
  });

  it('should use "parent" field for non-legacy invocations', async () => {
    // Mock useParams behavior
    jest.spyOn(ReactRouter, 'useParams').mockReturnValue({
      invocationId: 'inv-id',
      module: 'module',
      scheme: 'scheme',
      variant: 'hash',
      case: 'test-id',
    });

    renderComponent(false);

    await waitFor(() => {
      expect(screen.getByText('TestInfo')).toBeInTheDocument();
    });

    expect(mockBatchGetTestVariants).toHaveBeenCalledWith(
      expect.objectContaining({
        parent: 'rootInvocations/inv-id',
        // In some proto implementations, unused string fields default to empty string
        invocation: '',
      }),
    );
  });
});
