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

import Grid from '@mui/material/Grid';
import { useRef } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { SanitizedHtml } from '@/common/components/sanitized_html';
import { ArtifactTagScope } from '@/test_verdict/components/artifact_tags';

interface Props {
  summaryHtml: string;
  resultName: string;
  invId: string;
}

export function ResultSummary({ summaryHtml, resultName }: Props) {
  const mainRef = useRef<HTMLDivElement>(null);

  return (
    <RecoverableErrorBoundary>
      <Grid item ref={mainRef}>
        <ArtifactTagScope resultName={resultName}>
          <SanitizedHtml
            sx={{
              backgroundColor: 'var(--block-background-color)',
              paddingX: 1,
              maxHeight: '54vh',
              overflowX: 'auto',
              whiteSpace: 'pre-wrap',
              '& pre': {
                margin: 0,
                fontSize: '12px',
                whiteSpace: 'pre-wrap',
                overflowWrap: 'break-word',
              },
            }}
            html={summaryHtml}
          />
        </ArtifactTagScope>
      </Grid>
    </RecoverableErrorBoundary>
  );
}
