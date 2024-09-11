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

import { Box, CircularProgress } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { DateTime } from 'luxon';
import { useCallback, useRef } from 'react';

import { useChangepointsClient } from '@/analysis/hooks/prpc_clients';
import { OutputChangepointGroupSummary } from '@/analysis/types';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  ChangepointPredicate,
  QueryChangepointGroupSummariesRequest,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';
import { getRegressionDetailsURLPath } from '@/test_verdict/tools/url_utils';

import { RegressionFilters } from './regression_filters';
import { getWeek, RegressionPager } from './regression_pager';
import { RegressionTable } from './regression_table';

function getPredicate(searchParams: URLSearchParams) {
  const predicate = searchParams.get('cp') || '{}';
  return ChangepointPredicate.fromJSON(JSON.parse(predicate));
}

function predicateUpdater(newPredicate: ChangepointPredicate) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    const predicateStr = JSON.stringify(
      ChangepointPredicate.toJSON(newPredicate),
    );
    if (predicateStr === '{}') {
      searchParams.delete('cp');
    } else {
      searchParams.set('cp', predicateStr);
    }
    return searchParams;
  };
}

export interface RecentRegressionsProps {
  readonly project: string;
}

export function RecentRegressions({ project }: RecentRegressionsProps) {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const predicate = getPredicate(searchParams);
  const now = useRef(DateTime.now());
  const week = getWeek(searchParams, now.current);
  const client = useChangepointsClient();
  const { data, isLoading, isError, error } = useQuery(
    client.QueryChangepointGroupSummaries.query(
      QueryChangepointGroupSummariesRequest.fromPartial({
        project,
        predicate,
        beginOfWeek: week.toISO(),
      }),
    ),
  );

  if (isError) {
    throw error;
  }

  const getDetailsUrlPath = useCallback(
    (group: OutputChangepointGroupSummary) =>
      getRegressionDetailsURLPath({
        canonicalChangepoint: group.canonicalChangepoint,
        predicate,
      }),
    [predicate],
  );

  return (
    <>
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        sx={{ margin: '10px 20px' }}
      >
        <RegressionFilters
          predicate={predicate}
          onPredicateUpdate={(p) => setSearchParams(predicateUpdater(p))}
        />
      </Box>
      <RegressionPager now={now.current} />
      {isLoading ? (
        <Box display="flex" justifyContent="center" alignItems="center">
          <CircularProgress />
        </Box>
      ) : (
        <RegressionTable
          regressions={
            data.groupSummaries as readonly OutputChangepointGroupSummary[]
          }
          getDetailsUrlPath={getDetailsUrlPath}
        />
      )}
    </>
  );
}
