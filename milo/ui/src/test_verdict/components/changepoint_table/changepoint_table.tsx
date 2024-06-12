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

import { Box, CircularProgress, SxProps, Theme, styled } from '@mui/material';
import { UseQueryOptions, useQueries } from '@tanstack/react-query';
import { chunk } from 'lodash-es';

import { useTestVariantBranchesClient } from '@/analysis/hooks/prpc_clients';
import {
  OutputBatchGetTestVariantBranchResponse,
  ParsedTestVariantBranchName,
  TestVariantBranchDef,
} from '@/analysis/types';
import {
  BatchGetTestVariantBranchRequest,
  BatchGetTestVariantBranchResponse,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { getCriticalVariantKeys } from '@/test_verdict/tools/variant_utils';

import { Body } from './body';
import { ChangepointTableContextProvider } from './provider';
import { SidePanel } from './side_panel';
import { TopAxis } from './top_axis';
import { TopLabel } from './top_label';

const Container = styled(Box)`
  display: grid;
  grid-template-columns: auto 1fr;
  grid-template-areas:
    'top-label top-axis'
    'side-panel body';
  // Use a negative margin to make borders collapse between items collapse.
  & > * {
    margin: -1px;
  }
`;

export interface ChangepointTableProps {
  readonly testVariantBranchDefs: readonly TestVariantBranchDef[];
  readonly sx?: SxProps<Theme>;
}

export function ChangepointTable({
  testVariantBranchDefs,
  sx,
}: ChangepointTableProps) {
  const client = useTestVariantBranchesClient();
  type QueryOpts = UseQueryOptions<
    BatchGetTestVariantBranchResponse,
    unknown,
    OutputBatchGetTestVariantBranchResponse
  >;
  const queryResults = useQueries({
    queries: chunk(testVariantBranchDefs, 100).map<QueryOpts>((batch) => ({
      ...client.BatchGet.query(
        BatchGetTestVariantBranchRequest.fromPartial({
          names: Object.freeze(
            batch.map((tvb) => ParsedTestVariantBranchName.toString(tvb)),
          ),
        }),
      ),
      select: (data) => data as OutputBatchGetTestVariantBranchResponse,
    })),
  });
  for (const { isError, error } of queryResults) {
    if (isError) {
      throw error;
    }
  }

  const testVariantBranches = queryResults.some((q) => !q.data)
    ? []
    : queryResults.flatMap((q) => q.data!.testVariantBranches);

  const commits = testVariantBranches.flatMap((tvb) =>
    tvb.segments.flatMap((seg) => [
      seg.startPosition,
      seg.endPosition,
      ...(seg.hasStartChangepoint
        ? [seg.startPositionLowerBound99th, seg.startPositionUpperBound99th]
        : []),
    ]),
  );
  const criticalCommits = [...new Set(commits).values()].sort(
    (c1, c2) => parseInt(c2) - parseInt(c1),
  );

  const criticalVariantKeys = getCriticalVariantKeys(
    testVariantBranchDefs
      .map((tvb) => tvb.variant)
      .filter((v) => v !== undefined)
      // Do a `.map()` for type casting. In a future TypeScript version, tsc
      // will be able to infer this.
      .map((v) => v!),
  );

  if (queryResults.some((q) => q.isLoading)) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center">
        <CircularProgress />
      </Box>
    );
  }

  return (
    <ChangepointTableContextProvider
      criticalCommits={criticalCommits}
      criticalVariantKeys={criticalVariantKeys}
      testVariantBranchCount={testVariantBranchDefs.length}
    >
      <Container sx={sx}>
        <TopAxis />
        <TopLabel />
        <SidePanel testVariantBranches={testVariantBranchDefs} />
        <Body testVariantBranches={testVariantBranches} />
      </Container>
    </ChangepointTableContextProvider>
  );
}
