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
import { useState } from 'react';

import { useChangepointsClient } from '@/analysis/hooks/prpc_clients';
import {
  ChangepointPredicate,
  QueryChangepointGroupSummariesRequest,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';
import { OutputChangepointGroupSummary } from '@/test_verdict/types';

import { RegressionFilters } from './regression_filters';
import { RegressionTable } from './regression_table';

export interface RecentRegressionsProps {
  readonly project: string;
}

export function RecentRegressions({ project }: RecentRegressionsProps) {
  const [predicate, setPredicate] = useState(() =>
    ChangepointPredicate.create(),
  );

  const client = useChangepointsClient();
  const { data, isLoading, isError, error } = useQuery(
    client.QueryChangepointGroupSummaries.query(
      QueryChangepointGroupSummariesRequest.fromPartial({
        project,
        predicate,
      }),
    ),
  );

  if (isError) {
    throw error;
  }

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
          onPredicateUpdate={(p) => setPredicate(p)}
        />
      </Box>
      {isLoading ? (
        <Box display="flex" justifyContent="center" alignItems="center">
          <CircularProgress />
        </Box>
      ) : (
        <RegressionTable
          regressions={
            data.groupSummaries as readonly OutputChangepointGroupSummary[]
          }
        />
      )}
    </>
  );
}
