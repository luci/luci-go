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

import { Box, Typography } from '@mui/material';
import * as Diff2Html from 'diff2html';
import { useMemo } from 'react';

import { SanitizedHtml } from '@/common/components/sanitized_html';

interface TextDiffArtifactViewProps {
  content: string;
}

export function TextDiffArtifactView({ content }: TextDiffArtifactViewProps) {
  const diffHtml = useMemo(() => {
    if (!content) {
      return '';
    }
    return Diff2Html.html(content, {
      drawFileList: false,
      outputFormat: 'side-by-side',
      matching: 'lines',
    });
  }, [content]);

  if (!diffHtml) {
    return (
      <Typography sx={{ mt: 1 }} color="text.secondary">
        Text diff artifact is empty or content not available.
      </Typography>
    );
  }

  return (
    <>
      {/*
        In test environment (JSDOM), fetching external stylesheets can cause
        React to suspend indefinitely or fail the commit phase, leaving the
        component in a loading state. We skip this link tag during tests.
      */}
      {process.env.NODE_ENV !== 'test' && (
        <link
          rel="stylesheet"
          href="https://cdn.jsdelivr.net/npm/diff2html/bundles/css/diff2html.min.css"
          precedence="default"
        />
      )}
      <Box sx={{ fontFamily: 'monospace', fontSize: '0.8rem', px: 3 }}>
        <SanitizedHtml html={diffHtml} />
      </Box>
    </>
  );
}
