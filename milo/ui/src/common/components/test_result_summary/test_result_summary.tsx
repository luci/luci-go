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

import { memo } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { SanitizedHtml } from '@/common/components/sanitized_html';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { ArtifactTagScope } from '@/test_verdict/components/artifact_tags';

export interface TestResultSummaryProps {
  readonly testResult: TestResult;
}

export const TestResultSummary = memo(function TestResultSummary({
  testResult,
}: TestResultSummaryProps) {
  return (
    <RecoverableErrorBoundary>
      <ArtifactTagScope resultName={testResult.name}>
        <SanitizedHtml
          sx={{
            backgroundColor: 'var(--block-background-color)',
            padding: '5px',
            maxHeight: '54vh',
            overflowX: 'auto',
            whiteSpace: 'pre-wrap',
            '&>p:first-of-type': {
              marginTop: 0,
            },
            '&>p:last-of-type': {
              marginBottom: 0,
            },
            '& pre': {
              margin: 0,
              fontSize: '12px',
              whiteSpace: 'pre-wrap',
              overflowWrap: 'break-word',
            },
          }}
          html={testResult.summaryHtml}
        />
      </ArtifactTagScope>
    </RecoverableErrorBoundary>
  );
});
