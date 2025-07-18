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

import { Box, Link } from '@mui/material';

import {
  PageSummaryLine,
  SummaryLineItem,
} from '@/common/components/page_summary_line';
import { PageTitle } from '@/common/components/page_title';
import { OutputTestVerdict } from '@/common/types/verdict';
import { TestVariantBranch } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import {
  useInvocation,
  useProject,
  useTestVariant,
} from '@/test_investigation/context';
import {
  getCommitGitilesUrlFromInvocation,
  getCommitInfoFromInvocation,
} from '@/test_investigation/utils/test_info_utils';

import { useTestVariantBranch } from './context';
import { TestInfoBreadcrumbs } from './test_info_breadcrumbs';

export function TestInfoHeader() {
  const testVariant = useTestVariant();
  const invocation = useInvocation();
  const project = useProject();
  const testVariantBranch = useTestVariantBranch();

  const testDisplayName = testVariant.testMetadata?.name || testVariant.testId;
  const commitInfo = getCommitInfoFromInvocation(invocation);
  const originalCommitLink = getCommitGitilesUrlFromInvocation(invocation);

  const blamelistCommitLink = constructBlamelistCommitLink(
    project,
    testVariant,
    testVariantBranch,
    invocation,
  );

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1, pl: 3 }}>
      <TestInfoBreadcrumbs
        invocation={invocation.name}
        testIdStructured={testVariant?.testIdStructured || undefined}
      />
      <PageTitle viewName="Test case" resourceName={testDisplayName} />
      <PageSummaryLine>
        {Object.entries(testVariant.variant?.def || {}).map(([key, value]) => (
          <SummaryLineItem key={key} label={key}>
            {value}
          </SummaryLineItem>
        ))}
        <SummaryLineItem label="Commit">
          <Link
            href={blamelistCommitLink || originalCommitLink}
            target="_blank"
            rel="noopener noreferrer"
          >
            {commitInfo}
          </Link>
        </SummaryLineItem>
      </PageSummaryLine>
    </Box>
  );
}

function constructBlamelistCommitLink(
  project: string | undefined,
  testVariant: OutputTestVerdict,
  testVariantBranch: TestVariantBranch | null | undefined,
  invocation: Invocation,
): string | undefined {
  // TODO: get this refhash from the invocation rather than the testVariantBranch once it is populated by the backend.
  const refHash = testVariantBranch?.refHash;
  const commitPosition =
    invocation.sourceSpec?.sources?.gitilesCommit?.position;

  if (
    project &&
    testVariant.testId &&
    testVariant.variantHash &&
    refHash &&
    commitPosition
  ) {
    const encodedTestId = encodeURIComponent(testVariant.testId);
    const baseUrl = `/ui/labs/p/${project}/tests/${encodedTestId}/variants/${testVariant.variantHash}/refs/${refHash}/blamelist`;
    return `${baseUrl}?expand=CP-${commitPosition}#CP-${commitPosition}`;
  }
  return undefined;
}
