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

import { Box, Skeleton, styled } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useRef, useState } from 'react';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { DotSpinner } from '@/generic_libs/components/dot_spinner';
import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from '@/generic_libs/components/expandable_entry';
import { CompactClList } from '@/gitiles/components/compact_cl_list';
import { GerritChange } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { BatchGetTestVariantsRequest_TestVariantIdentifier } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TestVerdict_StatusOverride } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';
import { InvIdLink } from '@/test_verdict/components/inv_id_link';
import { VerdictStatusIcon } from '@/test_verdict/components/verdict_status_icon';
import { OutputBatchGetTestVariantResponse } from '@/test_verdict/types';

import { RESULT_LIMIT } from './constants';
import { EntryContent } from './entry_content';
import { VerdictAssociatedBugsBadge } from './verdict_associated_bugs_badge';

const CompactClBadge = styled(CompactClList)`
  margin: 0;
  background-color: #b7b7b7;
  width: 100%;
  box-sizing: border-box;
  overflow: hidden;
  text-overflow: ellipsis;
  vertical-align: middle;
  padding: 0.25em 0.4em;
  font-size: 75%;
  font-weight: 700;
  line-height: 16px;
  text-align: center;
  white-space: nowrap;
  border-radius: 0.25rem;
  color: white;

  & > a {
    color: white;
    text-decoration: none;
    &:before {
      content: 'cl/';
    }
  }
`;

export interface TestVerdictEntryProps {
  readonly project: string;
  readonly testId: string;
  readonly variantHash: string;
  readonly invocationId: string;
  readonly changes?: readonly Omit<GerritChange, 'project'>[];
  readonly defaultExpanded?: boolean;
}

export function TestVerdictEntry({
  project,
  testId,
  variantHash,
  invocationId,
  changes = [],
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
  const { data, isPending, isError, error } = useQuery({
    ...client.BatchGetTestVariants.query({
      invocation: 'invocations/' + invocationId,
      testVariants: [
        BatchGetTestVariantsRequest_TestVariantIdentifier.fromPartial({
          testId,
          variantHash,
        }),
      ],
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
          <VerdictStatusIcon
            statusV2={testVerdict.statusV2}
            // Deliberately do not set statusOverride because it does not make sense to show status
            // overrides in this context. This is because in all places this component is used,
            // we are trying to assess the health of the test, not the CL under test.
            statusOverride={TestVerdict_StatusOverride.NOT_OVERRIDDEN}
          />
        ) : (
          <Skeleton variant="circular" height={24} width={24} />
        )}
        <Box>
          <InvIdLink invId={invocationId} />
          {!isPending && !expanded && testVerdict && (
            <>
              {' '}
              <VerdictAssociatedBugsBadge
                project={project}
                verdict={testVerdict}
              />
            </>
          )}
          <CompactClBadge
            changes={changes}
            maxInlineCount={2}
            sx={{ marginLeft: '3px' }}
          />
        </Box>
      </ExpandableEntryHeader>
      <ExpandableEntryBody>
        {isPending ? (
          <DotSpinner />
        ) : (
          <EntryContent project={project} verdict={data.testVariants[0]} />
        )}
      </ExpandableEntryBody>
    </ExpandableEntry>
  );
}
