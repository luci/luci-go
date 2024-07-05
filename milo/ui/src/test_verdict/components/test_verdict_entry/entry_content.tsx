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

import { SanitizedHtml } from '@/common/components/sanitized_html';
import { TestResultEntry } from '@/test_verdict/components/test_result_entry';
import { getSuggestedResultIndex } from '@/test_verdict/tools/test_result_utils';
import { OutputTestVerdict } from '@/test_verdict/types';

import { RESULT_LIMIT } from './constants';

export interface EntryContentProps {
  readonly project: string;
  readonly verdict: OutputTestVerdict;
}

export function EntryContent({ project, verdict }: EntryContentProps) {
  const expandResultIndex = getSuggestedResultIndex(verdict.results);

  return (
    <>
      {verdict.exonerations.map((e) => (
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
      {verdict.results.length === RESULT_LIMIT && (
        <Alert severity="warning" sx={{ margin: '10px 5px' }}>
          Only the first {RESULT_LIMIT} results are displayed.
        </Alert>
      )}
      {verdict.results.map((r, i) => (
        <TestResultEntry
          key={r.result.name}
          index={i}
          project={project}
          testResult={r.result}
          testId={verdict.testId}
          initExpanded={i === expandResultIndex}
        />
      ))}
    </>
  );
}
