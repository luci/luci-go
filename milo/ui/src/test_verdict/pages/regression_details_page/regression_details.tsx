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

import { Box, CircularProgress, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import {
  OutputChangepoint,
  ParsedTestVariantBranchName,
} from '@/analysis/types';
import { useChangepointsClient } from '@/common/hooks/prpc_clients';
import {
  ChangepointPredicate,
  QueryChangepointsInGroupRequest,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';
import {
  BlamelistStateProvider,
  ChangepointTable,
} from '@/test_verdict/components/changepoint_table';

import { RegressionDetailsDialog } from './regression_details_dialog';

export interface RegressionDetailsProps {
  readonly testVariantBranch: ParsedTestVariantBranchName;
  readonly nominalStartPosition: string;
  readonly startHour: string;
  readonly predicate: ChangepointPredicate;
}

export function RegressionDetails({
  testVariantBranch,
  nominalStartPosition,
  startHour,
  predicate,
}: RegressionDetailsProps) {
  const client = useChangepointsClient();
  const { data, isPending, isError, error } = useQuery({
    ...client.QueryChangepointsInGroup.query(
      QueryChangepointsInGroupRequest.fromPartial({
        project: testVariantBranch.project,
        groupKey: {
          testId: testVariantBranch.testId,
          variantHash: testVariantBranch.variantHash,
          refHash: testVariantBranch.refHash,
          nominalStartPosition,
          startHour,
        },
        predicate,
      }),
    ),
    select: (data) => data.changepoints as readonly OutputChangepoint[],
    // Refetching causes the group to be updated and the page to flash.
    // Given that the grouping is not precise in the first place. Disable
    // refetch on window focus.
    refetchOnWindowFocus: false,
  });

  if (isError) {
    throw error;
  }

  if (isPending) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center">
        <CircularProgress />
      </Box>
    );
  }

  // TODO(b/321110247): Display all the test variant branches in a table with
  // all the associated changepoints.
  return (
    <BlamelistStateProvider>
      <Typography variant="h6" sx={{ padding: '10px' }}>
        Regression group: {testVariantBranch.testId} ({data.length} test
        variants)
      </Typography>
      <ChangepointTable
        testVariantBranchDefs={data}
        sx={{ paddingLeft: '10px', paddingRight: '10px' }}
      />
      <RegressionDetailsDialog />
    </BlamelistStateProvider>
  );
}
