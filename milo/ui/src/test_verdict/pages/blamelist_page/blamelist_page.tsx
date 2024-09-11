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
import { useLocation, useParams } from 'react-router-dom';

import { useTestVariantBranchesClient } from '@/analysis/hooks/prpc_clients';
import {
  OutputTestVariantBranch,
  ParsedTestVariantBranchName,
} from '@/analysis/types';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { BatchGetTestVariantBranchRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { BlamelistTable } from '@/test_verdict/components/blamelist_table';
import { TestVariantBranchId } from '@/test_verdict/components/test_variant_branch_id';

export function BlamelistPage() {
  const { project, testId, variantHash, refHash } = useParams();
  if (!project || !testId || !variantHash || !refHash) {
    throw new Error(
      'invariant violated: project, testId, variantHash, refHash should be set',
    );
  }

  const { hash: urlHash } = useLocation();

  const testVariantBranch = {
    project,
    testId,
    variantHash,
    refHash,
  };

  const client = useTestVariantBranchesClient();
  const { data, isLoading, isError, error } = useQuery({
    ...client.BatchGet.query(
      BatchGetTestVariantBranchRequest.fromPartial({
        names: [ParsedTestVariantBranchName.toString(testVariantBranch)],
      }),
    ),
    select: (data) => data.testVariantBranches[0] as OutputTestVariantBranch,
  });
  if (isError) {
    throw error;
  }

  if (isLoading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center">
        <CircularProgress />
      </Box>
    );
  }

  const lastCommitPosition = data.segments[0].endPosition;
  const firstSegment = data.segments[data.segments.length - 1];
  const firstCommitPosition = firstSegment.hasStartChangepoint
    ? firstSegment.startPositionLowerBound99th
    : firstSegment.startPosition;
  const focusCommitPosition = urlHash.match(/^#CP-\d+$/)
    ? // Normalize the commit position.
      String(
        Math.min(
          // +2 because the first item might be covered by the sticky top
          // header.
          parseInt(urlHash.slice('#CP-'.length)) + 2,
          parseInt(lastCommitPosition),
        ),
      )
    : undefined;

  return (
    <>
      <Box
        sx={{
          borderBottom: 'solid 1px var(--divider-color)',
          backgroundColor: 'var(--block-background-color)',
          fontSize: '16px',
        }}
      >
        <TestVariantBranchId
          gitilesRef={data.ref.gitiles}
          testId={data.testId}
          variant={data.variant}
        />
      </Box>
      <BlamelistTable
        lastCommitPosition={lastCommitPosition}
        firstCommitPosition={firstCommitPosition}
        testVariantBranch={data}
        focusCommitPosition={focusCommitPosition}
        useWindowScroll
      />
    </>
  );
}

export function Component() {
  const { project } = useParams();
  if (!project) {
    throw new Error('invariant violated: project should be set');
  }

  return (
    <>
      {/* See the documentation for `<LoginPage />` for why we handle error this
       ** way.
       **/}
      <PageMeta
        project={project}
        title="Blamelist"
        selectedPage={UiPage.Blamelist}
      />
      <RecoverableErrorBoundary key="blamelist">
        <BlamelistPage />
      </RecoverableErrorBoundary>
    </>
  );
}
