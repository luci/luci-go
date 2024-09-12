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

import { Box, Skeleton } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useRef, useState } from 'react';

import { DotSpinner } from '@/generic_libs/components/dot_spinner';
import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';
import { InvIdLink } from '@/test_verdict/components/inv_id_link';
import { VerdictStatusIcon } from '@/test_verdict/components/verdict_status_icon';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';
import { OutputBatchGetTestVariantResponse } from '@/test_verdict/types';

import { RESULT_LIMIT } from './constants';
import { EntryContent } from './entry_content';
import { VerdictAssociatedBugsBadge } from './verdict_associated_bugs_badge';

export interface TestVerdictEntryProps {
  readonly project: string;
  readonly testId: string;
  readonly variantHash: string;
  readonly invocationId: string;
  readonly defaultExpanded?: boolean;
}

export function TestVerdictEntry({
  project,
  testId,
  variantHash,
  invocationId,
  defaultExpanded = false,
}: TestVerdictEntryProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);

  // Whenever the default expanded state changes, update the expanded state to
  // match.
  const lastDefaultExpanded = useRef(defaultExpanded);
  if (lastDefaultExpanded.current !== defaultExpanded) {
    lastDefaultExpanded.current = defaultExpanded;
    setExpanded(defaultExpanded);
  }

  const client = useResultDbClient();
  const { data, isLoading, isError, error } = useQuery({
    ...client.BatchGetTestVariants.query({
      invocation: 'invocations/' + invocationId,
      testVariants: [{ testId, variantHash }],
      resultLimit: RESULT_LIMIT,
    }),
    select: (data) => data as OutputBatchGetTestVariantResponse,
  });
  if (isError) {
    throw error;
  }

  const testVerdict = data?.testVariants[0];

  return (
    <ExpandableEntry expanded={expanded}>
      <ExpandableEntryHeader
        onToggle={() => setExpanded(!expanded)}
        sx={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '5px' }}
      >
        {testVerdict ? (
          <VerdictStatusIcon status={testVerdict.status} />
        ) : (
          <Skeleton variant="circular" height={24} width={24} />
        )}
        <Box>
          <InvIdLink invId={invocationId} />
          {!isLoading && !expanded && testVerdict && (
            <>
              {' '}
              <VerdictAssociatedBugsBadge
                project={project}
                verdict={testVerdict}
              />
            </>
          )}
        </Box>
      </ExpandableEntryHeader>
      <ExpandableEntryBody>
        {isLoading ? (
          <DotSpinner />
        ) : (
          <EntryContent project={project} verdict={data.testVariants[0]} />
        )}
      </ExpandableEntryBody>
    </ExpandableEntry>
  );
}
