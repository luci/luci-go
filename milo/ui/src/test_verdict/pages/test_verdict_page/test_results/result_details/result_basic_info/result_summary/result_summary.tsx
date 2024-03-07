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
import { useEffect, useRef } from 'react';

import { SanitizedHtml } from '@/common/components/sanitized_html';
import { TextArtifactEvent } from '@/common/components/text_artifact';
import { ON_TEST_RESULT_DATA_READY } from '@/common/constants/event';

import '@/common/components/text_artifact';

interface Props {
  summaryHtml: string;
  resultName: string;
  invId: string;
}

export function ResultSummary({ summaryHtml, resultName, invId }: Props) {
  const mainRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleGetTextArtifactData(e: Event) {
      (e as CustomEvent<TextArtifactEvent>).detail.setData(
        `invocations/${invId}`,
        resultName,
      );
    }

    if (mainRef.current) {
      const elem = mainRef.current;
      elem.addEventListener(
        ON_TEST_RESULT_DATA_READY,
        handleGetTextArtifactData,
      );

      return () => {
        elem.removeEventListener(
          ON_TEST_RESULT_DATA_READY,
          handleGetTextArtifactData,
        );
      };
    }
    return;
  }, [invId, resultName]);

  return (
    <Grid item ref={mainRef}>
      <SanitizedHtml
        // Key is required to force a remount of the text-artifact element
        // to force refetching for the new result data.
        key={resultName}
        sx={{
          backgroundColor: 'var(--block-background-color)',
          paddingX: 1,
          maxHeight: '500px',
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
    </Grid>
  );
}
