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

import { Alert } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import { OutputTestVerdict } from '@/analysis/types';
import { SanitizedHtml } from '@/common/components/sanitized_html';
import { DotSpinner } from '@/generic_libs/components/dot_spinner';
import { TestResultEntry } from '@/test_verdict/components/test_result_entry';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';
import { OutputBatchGetTestVariantResponse } from '@/test_verdict/types';

const RESULT_LIMIT = 10;

export interface EntryContentProps {
  readonly verdict: OutputTestVerdict;
}

export function EntryContent({ verdict }: EntryContentProps) {
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

  if (isLoading) {
    return <DotSpinner />;
  }

  const testVerdict = data.testVariants[0];

  return (
    <>
      {testVerdict.exonerations.map((e) => (
        <SanitizedHtml
          key={e.name}
          html={
            e.explanationHtml ||
            'This test verdict had unexpected results, but was exonerated (reason not provided).'
          }
          sx={{
            backgroundColor: 'var(--block-background-color)',
            padding: '5px 10px',
            margin: '10px 5px',
          }}
        />
      ))}
      {testVerdict.results.length === RESULT_LIMIT && (
        <Alert severity="warning" sx={{ margin: '10px 5px' }}>
          Only the first {RESULT_LIMIT} results are displayed.
        </Alert>
      )}
      {testVerdict.results.map((r, i) => (
        <TestResultEntry key={r.result.name} index={i} testResult={r.result} />
      ))}
    </>
  );
}
