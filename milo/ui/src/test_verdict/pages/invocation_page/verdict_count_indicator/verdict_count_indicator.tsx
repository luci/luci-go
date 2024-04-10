// Copyright 2024 The LUCI Authors.
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

import { Box, CircularProgress, styled } from '@mui/material';
import { useInfiniteQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import { VERDICT_STATUS_DISPLAY_MAP } from '@/common/constants/test';
import { formatNum } from '@/generic_libs/tools/string_utils';
import { QueryTestVariantsRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import {
  VERDICT_STATUS_COLOR_MAP,
  VERDICT_STATUS_ICON_MAP,
} from '@/test_verdict/constants/verdict';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';
import { SpecifiedTestVerdictStatus } from '@/test_verdict/types';

export const QUERY_TEST_VERDICT_PAGE_SIZE = 10000;

const Container = styled(Box)`
  color: white;
  display: inline-block;
  padding: 0.1em;
  padding-top: 0.2em;
  margin-left: 3px;
  font-size: 75%;
  font-weight: 700;
  line-height: 13px;
  text-align: center;
  white-space: nowrap;
  vertical-align: bottom;
  border-radius: 0.25rem;
  margin-bottom: 2px;
  box-sizing: border-box;
  width: 30px;
`;

export interface VerdictCountIndicatorProps {
  readonly invName: string;
}

export function VerdictCountIndicator({ invName }: VerdictCountIndicatorProps) {
  const client = useResultDbClient();
  const { data, error, isError, isLoading, hasNextPage } = useInfiniteQuery(
    client.QueryTestVariants.queryPaged(
      QueryTestVariantsRequest.fromPartial({
        invocations: Object.freeze([invName]),
        pageSize: QUERY_TEST_VERDICT_PAGE_SIZE,
      }),
    ),
  );

  if (isError) {
    throw error;
  }

  const firstPage = data?.pages[0];
  const hasVerdicts = (firstPage?.testVariants.length || 0) !== 0;
  const worstStatus = (firstPage?.testVariants[0]?.status ||
    TestVariantStatus.UNSPECIFIED) as
    | SpecifiedTestVerdictStatus
    | TestVariantStatus.UNSPECIFIED;
  const { worstStatusCount, countedAll } = useMemo(
    () => {
      // Not interested in the count when there are no test verdict worse than
      // being exonerated.
      if (
        worstStatus === TestVariantStatus.UNSPECIFIED ||
        worstStatus >= TestVariantStatus.EXONERATED
      ) {
        return { worstStatusCount: 0, countedAll: true };
      }

      let worstStatusCount = 0;
      for (const tv of firstPage?.testVariants || []) {
        // Verdicts are sorted by their status. Do not need to check further
        // once we found a different status.
        if (tv.status > worstStatus) {
          return { worstStatusCount, countedAll: true };
        }
        worstStatusCount += 1;
      }
      return {
        worstStatusCount,
        // If the actual page size is lower than the requested one, this means
        // the all the non-expected verdicts have already been included in this
        // page.
        countedAll:
          (firstPage?.testVariants.length || 0) < QUERY_TEST_VERDICT_PAGE_SIZE,
      };
    },
    // Only count the first page so this is not re-computed every time a new
    // page loads.
    [firstPage, worstStatus],
  );

  if (isLoading) {
    return (
      <Container>
        <CircularProgress size={15} />
      </Container>
    );
  }

  if (!hasVerdicts || worstStatus === TestVariantStatus.UNSPECIFIED) {
    return <Container />;
  }

  if (worstStatus >= TestVariantStatus.EXONERATED) {
    return <Container>{VERDICT_STATUS_ICON_MAP[worstStatus]}</Container>;
  }

  const hasMore = Boolean(!countedAll && hasNextPage);

  return (
    <Container
      data-testid="verdict-count-indicator"
      title={`${formatNum(worstStatusCount, hasMore)} ${
        VERDICT_STATUS_DISPLAY_MAP[worstStatus]
      } test verdict(s)`}
      sx={{ backgroundColor: VERDICT_STATUS_COLOR_MAP[worstStatus] }}
    >
      {formatNum(worstStatusCount, hasMore, 99)}
    </Container>
  );
}
