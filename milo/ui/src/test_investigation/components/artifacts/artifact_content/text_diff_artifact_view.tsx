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

import { Box, CircularProgress, Typography } from '@mui/material';
import * as Diff2Html from 'diff2html';
import { useMemo } from 'react';

import { SanitizedHtml } from '@/common/components/sanitized_html';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { useFetchArtifactContentQuery } from '@/test_investigation/hooks/queries';

interface TextDiffArtifactViewProps {
  artifact: Artifact;
}

export function TextDiffArtifactView({ artifact }: TextDiffArtifactViewProps) {
  const {
    data: artifactContentData,
    isPending: isLoadingArtifactContent,
    error,
  } = useFetchArtifactContentQuery({
    artifact,
    artifactContentQueryEnabled: !!artifact.fetchUrl,
  });

  const diffHtml = useMemo(() => {
    if (
      !artifactContentData?.data ||
      typeof artifactContentData.data !== 'string'
    ) {
      return '';
    }
    return Diff2Html.html(artifactContentData.data, {
      drawFileList: false,
      outputFormat: 'side-by-side',
      matching: 'lines',
    });
  }, [artifactContentData]);

  if (isLoadingArtifactContent) {
    return (
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          p: 1,
        }}
      >
        <CircularProgress size={24} />
        <Typography sx={{ ml: 1 }}>Loading Text Diff...</Typography>
      </Box>
    );
  }

  if (error) {
    return (
      <Typography sx={{ mt: 1, color: 'error.main' }}>
        Error loading text diff: {error.message}
      </Typography>
    );
  }

  if (!diffHtml) {
    return (
      <Typography sx={{ mt: 1 }} color="text.secondary">
        Text diff artifact is empty or content not available.
      </Typography>
    );
  }

  return (
    <>
      <link
        rel="stylesheet"
        href="https://cdn.jsdelivr.net/npm/diff2html/bundles/css/diff2html.min.css"
        precedence="default"
      />
      <Box sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
        <SanitizedHtml html={diffHtml} />
      </Box>
    </>
  );
}
