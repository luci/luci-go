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

import { Box } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';

import { OutputTestVerdict } from '@/analysis/types';
import { DotSpinner } from '@/generic_libs/components/dot_spinner';
import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';
import { VerdictStatusIcon } from '@/test_verdict/components/verdict_status_icon';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';
import {
  OutputBatchGetTestVariantResponse,
  SpecifiedTestVerdictStatus,
} from '@/test_verdict/types';

import { RESULT_LIMIT } from './constants';
import { EntryContent } from './entry_content';
import { VerdictAssociatedBugsBadge } from './verdict_associated_bugs_badge';

export interface TestVerdictEntryProps {
  readonly verdict: OutputTestVerdict;
}

export function TestVerdictEntry({ verdict }: TestVerdictEntryProps) {
  const [expanded, setExpanded] = useState(false);

  const client = useResultDbClient();
  const { data, isLoading, isError, error } = useQuery({
    ...client.BatchGetTestVariants.query({
      invocation: 'invocations/' + verdict.invocationId,
      testVariants: [
        { testId: verdict.testId, variantHash: verdict.variantHash },
      ],
      resultLimit: RESULT_LIMIT,
    }),
    select: (data) => data as OutputBatchGetTestVariantResponse,
  });
  if (isError) {
    throw error;
  }

  return (
    <ExpandableEntry expanded={expanded}>
      <ExpandableEntryHeader
        onToggle={() => setExpanded(!expanded)}
        sx={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '5px' }}
      >
        <VerdictStatusIcon
          status={SpecifiedTestVerdictStatus.fromAnalysis(verdict.status)}
        />
        <Box>
          {verdict.invocationId}
          {!isLoading && !expanded && (
            <>
              {' '}
              <VerdictAssociatedBugsBadge verdict={data.testVariants[0]} />
            </>
          )}
        </Box>
      </ExpandableEntryHeader>
      <ExpandableEntryBody>
        {isLoading ? (
          <DotSpinner />
        ) : (
          <EntryContent verdict={data.testVariants[0]} />
        )}
      </ExpandableEntryBody>
    </ExpandableEntry>
  );
}
