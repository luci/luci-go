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

import { useInfiniteQuery } from '@tanstack/react-query';
import { render } from '@testing-library/react';

import { useFeatureFlag } from '@/common/feature_flags/context';
import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Invocation_State } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import {
  useInvocation,
  useIsLegacyInvocation,
  useRawInvocationId,
} from '@/test_investigation/context/context';
import { TEST_AGGREGATION_IN_INVOCATION_FLAG } from '@/test_investigation/pages/features';

import { TestTab } from './test_tab';

jest.mock('@tanstack/react-query', () => ({
  useInfiniteQuery: jest.fn(),
}));

jest.mock('@/generic_libs/components/routed_tabs', () => ({
  useDeclareTabId: jest.fn(),
}));

jest.mock('react-router', () => ({
  useNavigate: () => jest.fn(),
}));

jest.mock('@/common/feature_flags/context', () => {
  const actual = jest.requireActual('@/common/feature_flags/context');
  return {
    ...actual,
    useFeatureFlag: jest.fn(),
  };
});

jest.mock('@/common/hooks/prpc_clients', () => ({
  useResultDbClient: jest.fn(),
}));

jest.mock('@/generic_libs/hooks/synced_search_params', () => ({
  useSyncedSearchParams: jest.fn(),
}));

jest.mock('@/test_investigation/context/context', () => ({
  useInvocation: jest.fn(),
  useRawInvocationId: jest.fn(),
  useIsLegacyInvocation: jest.fn(),
}));

jest.mock('@/test_investigation/components/invocation_page', () => ({
  InvocationCounts: () => <div data-testid="invocation-counts" />,
  TestVariantsTable: () => <div data-testid="test-variants-table" />,
}));

jest.mock(
  '@/test_investigation/components/test_aggregation_viewer/test_aggregation_viewer',
  () => ({
    TestAggregationViewer: () => <div data-testid="test-aggregation-viewer" />,
  }),
);

describe('TestTab', () => {
  const mockQueryPaged = jest.fn();
  const mockUseInfiniteQuery = useInfiniteQuery as jest.Mock;
  const mockUseFeatureFlag = useFeatureFlag as jest.Mock;
  const mockUseResultDbClient = useResultDbClient as jest.Mock;
  const mockUseSyncedSearchParams = useSyncedSearchParams as jest.Mock;
  const mockUseInvocation = useInvocation as jest.Mock;
  const mockUseRawInvocationId = useRawInvocationId as jest.Mock;
  const mockUseIsLegacyInvocation = useIsLegacyInvocation as jest.Mock;

  beforeEach(() => {
    jest.clearAllMocks();

    mockQueryPaged.mockImplementation((req) => ({
      queryKey: ['queryTestVariants', req],
      queryFn: jest.fn(),
    }));

    mockUseResultDbClient.mockReturnValue({
      QueryTestVariants: {
        queryPaged: mockQueryPaged,
      },
    });

    mockUseInfiniteQuery.mockReturnValue({
      data: undefined,
      isFetching: false,
      isSuccess: false,
      isPending: false,
      hasNextPage: false,
      fetchNextPage: jest.fn(),
      isFetchingNextPage: false,
    });

    mockUseRawInvocationId.mockReturnValue('build-12345');
    mockUseInvocation.mockReturnValue({
      name: 'invocations/build-12345',
      state: Invocation_State.ACTIVE,
    });
    mockUseIsLegacyInvocation.mockReturnValue(false);
    mockUseFeatureFlag.mockReturnValue(false);
  });

  describe('predicate filtering', () => {
    it('sets UNEXPECTED_MASK predicate by default when only unexpected statuses are selected', () => {
      mockUseSyncedSearchParams.mockReturnValue([
        new URLSearchParams(),
        jest.fn(),
      ]);

      render(<TestTab />);

      expect(mockQueryPaged).toHaveBeenCalledWith(
        expect.objectContaining({
          predicate: { status: TestVariantStatus.UNEXPECTED_MASK },
        }),
      );
    });

    it('sets UNEXPECTED_MASK predicate when status param has only unexpected statuses', () => {
      mockUseSyncedSearchParams.mockReturnValue([
        new URLSearchParams('status=failed,flaky'),
        jest.fn(),
      ]);

      render(<TestTab />);

      expect(mockQueryPaged).toHaveBeenCalledWith(
        expect.objectContaining({
          predicate: { status: TestVariantStatus.UNEXPECTED_MASK },
        }),
      );
    });

    it('omits predicate when status includes passed', () => {
      mockUseSyncedSearchParams.mockReturnValue([
        new URLSearchParams('status=failed,passed'),
        jest.fn(),
      ]);

      render(<TestTab />);

      expect(mockQueryPaged).toHaveBeenCalledWith(
        expect.objectContaining({
          predicate: undefined,
        }),
      );
    });

    it('omits predicate when status includes skipped', () => {
      mockUseSyncedSearchParams.mockReturnValue([
        new URLSearchParams('status=skipped'),
        jest.fn(),
      ]);

      render(<TestTab />);

      expect(mockQueryPaged).toHaveBeenCalledWith(
        expect.objectContaining({
          predicate: undefined,
        }),
      );
    });

    it('omits predicate when status is empty (all statuses selected)', () => {
      mockUseSyncedSearchParams.mockReturnValue([
        new URLSearchParams('status='),
        jest.fn(),
      ]);

      render(<TestTab />);

      expect(mockQueryPaged).toHaveBeenCalledWith(
        expect.objectContaining({
          predicate: undefined,
        }),
      );
    });
  });

  describe('query gating', () => {
    it('enables query for root invocation when aggregation flag is disabled', () => {
      mockUseSyncedSearchParams.mockReturnValue([
        new URLSearchParams(),
        jest.fn(),
      ]);
      mockUseFeatureFlag.mockImplementation((flag) =>
        flag === TEST_AGGREGATION_IN_INVOCATION_FLAG ? false : false,
      );
      mockUseIsLegacyInvocation.mockReturnValue(false);

      render(<TestTab />);

      expect(mockUseInfiniteQuery).toHaveBeenCalledWith(
        expect.objectContaining({
          enabled: true,
        }),
      );
    });

    it('disables query for root invocation when aggregation flag is enabled', () => {
      mockUseSyncedSearchParams.mockReturnValue([
        new URLSearchParams(),
        jest.fn(),
      ]);
      mockUseFeatureFlag.mockImplementation((flag) =>
        flag === TEST_AGGREGATION_IN_INVOCATION_FLAG ? true : false,
      );
      mockUseIsLegacyInvocation.mockReturnValue(false);

      render(<TestTab />);

      expect(mockUseInfiniteQuery).toHaveBeenCalledWith(
        expect.objectContaining({
          enabled: false,
        }),
      );
    });

    it('enables query for legacy invocation even when aggregation flag is enabled', () => {
      mockUseSyncedSearchParams.mockReturnValue([
        new URLSearchParams(),
        jest.fn(),
      ]);
      mockUseFeatureFlag.mockImplementation((flag) =>
        flag === TEST_AGGREGATION_IN_INVOCATION_FLAG ? true : false,
      );
      mockUseIsLegacyInvocation.mockReturnValue(true);

      render(<TestTab />);

      expect(mockUseInfiniteQuery).toHaveBeenCalledWith(
        expect.objectContaining({
          enabled: true,
        }),
      );
    });

    it('disables query when invocation is not yet loaded', () => {
      mockUseSyncedSearchParams.mockReturnValue([
        new URLSearchParams(),
        jest.fn(),
      ]);
      mockUseInvocation.mockReturnValue(null);

      render(<TestTab />);

      expect(mockUseInfiniteQuery).toHaveBeenCalledWith(
        expect.objectContaining({
          enabled: false,
        }),
      );
    });
  });
});
