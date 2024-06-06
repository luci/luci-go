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
import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import { useTestVariantBranchesClient } from '@/analysis/hooks/prpc_clients';
import {
  OutputTestVariantBranch,
  ParsedTestVariantBranchName,
  TestVariantBranchDef,
} from '@/analysis/types';
import { BatchGetTestVariantBranchRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { getCriticalVariantKeys } from '@/test_verdict/tools/variant_utils';

import { Body } from './body';
import { ChangepointTableContextProvider } from './context';
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
  readonly testVariantBranches: readonly TestVariantBranchDef[];
  readonly sx?: SxProps<Theme>;
}

export function ChangepointTable({
  testVariantBranches,
  sx,
}: ChangepointTableProps) {
  const client = useTestVariantBranchesClient();
  const { data, isError, error, isLoading } = useQuery({
    ...client.BatchGet.query(
      BatchGetTestVariantBranchRequest.fromPartial({
        names: Object.freeze(
          testVariantBranches.map((tvb) =>
            ParsedTestVariantBranchName.toString(tvb),
          ),
        ),
      }),
    ),
    select: (data) =>
      data.testVariantBranches as readonly OutputTestVariantBranch[],
  });
  if (isError) {
    throw error;
  }

  const criticalCommits = useMemo(() => {
    const commits = data?.flatMap((tvb) =>
      tvb.segments.flatMap((seg) => [
        seg.startPosition,
        seg.endPosition,
        ...(seg.hasStartChangepoint
          ? [seg.startPositionLowerBound99th, seg.startPositionUpperBound99th]
          : []),
      ]),
    );
    return [...new Set(commits).values()].sort(
      (c1, c2) => parseInt(c2) - parseInt(c1),
    );
  }, [data]);

  const criticalVariantKeys = useMemo(() => {
    return getCriticalVariantKeys(
      testVariantBranches
        .map((tvb) => tvb.variant)
        .filter((v) => v !== undefined)
        // Do a `.map()` for type casting. In a future TypeScript version, tsc
        // will be able to infer this.
        .map((v) => v!),
    );
  }, [testVariantBranches]);

  if (isLoading) {
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
      testVariantBranchCount={testVariantBranches.length}
    >
      <Container sx={sx}>
        <TopAxis />
        <TopLabel />
        <SidePanel testVariantBranches={testVariantBranches} />
        <Body testVariantBranches={data} />
      </Container>
    </ChangepointTableContextProvider>
  );
}
