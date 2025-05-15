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

import DescriptionIcon from '@mui/icons-material/Description';
import { Box, Button, CircularProgress, Typography } from '@mui/material';
import { JSX } from 'react';

import { sanitizeHTML } from '@/common/tools/sanitize_html/sanitize_html';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

import { CustomArtifactTreeNode, FetchedArtifactContent } from './types';

interface ArtifactContentViewProps {
  selectedArtifactForDisplay: CustomArtifactTreeNode | null;
  currentResult?: TestResult; // For summary HTML
  artifactContentData?: FetchedArtifactContent;
  isLoadingArtifactContent: boolean;
  invocationHasArtifacts: boolean;
}

export function ArtifactContentView({
  selectedArtifactForDisplay,
  currentResult,
  artifactContentData,
  isLoadingArtifactContent,
  invocationHasArtifacts,
}: ArtifactContentViewProps): JSX.Element {
  if (isLoadingArtifactContent) {
    return (
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100%',
        }}
      >
        <CircularProgress size={24} />
        <Typography sx={{ ml: 1 }}>Loading content...</Typography>
      </Box>
    );
  }

  if (selectedArtifactForDisplay?.isSummary) {
    const summaryHtml = currentResult?.summaryHtml || '';
    const sanitizedSummaryHtml = sanitizeHTML(summaryHtml);
    return (
      <Box
        dangerouslySetInnerHTML={{
          __html: sanitizedSummaryHtml,
        }}
      />
    );
  }

  if (
    selectedArtifactForDisplay?.artifact &&
    !selectedArtifactForDisplay.isSummary
  ) {
    const artifactPb = selectedArtifactForDisplay.artifact;
    return (
      <Box>
        <Typography variant="h6" gutterBottom>
          {selectedArtifactForDisplay.name}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          Content Type:{' '}
          {artifactContentData?.contentType || artifactPb.contentType || 'N/A'}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          Size:{' '}
          {/* Ensure sizeBytes is not null before accessing, use 'N/A' or similar if undefined */}
          {artifactPb.sizeBytes !== null
            ? `${artifactPb.sizeBytes} bytes`
            : 'N/A'}
        </Typography>
        {artifactPb.fetchUrl && (
          <Button
            variant="outlined"
            size="small"
            href={getRawArtifactURLPath(artifactPb.name)}
            target="_blank"
            rel="noopener noreferrer" // Good for security
            sx={{ my: 1 }}
            startIcon={<DescriptionIcon />}
          >
            Download Artifact
          </Button>
        )}

        {artifactContentData?.error ? (
          <Typography sx={{ mt: 1, color: 'error.main' }}>
            Error loading artifact content: {artifactContentData.error.message}
          </Typography>
        ) : artifactContentData?.isText ? (
          artifactContentData.data !== null &&
          artifactContentData.data !== '' ? (
            <Box
              component="pre"
              sx={{
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-all',
                maxHeight: '400px',
                overflowY: 'auto',
                bgcolor: 'grey.100',
                p: 1,
                borderRadius: 1,
                mt: 1,
                fontSize: '0.8rem',
              }}
            >
              {artifactContentData.data}{' '}
            </Box>
          ) : (
            <Typography sx={{ mt: 1 }} color="text.secondary">
              Text artifact is empty.
            </Typography>
          )
        ) : artifactPb.contentType ? (
          <Typography sx={{ mt: 1 }} color="text.secondary">
            Preview not available for content type:{' '}
            {artifactContentData?.contentType || artifactPb.contentType}.
          </Typography>
        ) : (
          <Typography sx={{ mt: 1 }} color="text.secondary">
            Preview not available (unknown content type).
          </Typography>
        )}
      </Box>
    );
  }

  if (currentResult || invocationHasArtifacts) {
    return (
      <Typography color="text.secondary">
        Select an artifact from the tree to view its details.
      </Typography>
    );
  }

  return (
    <Typography color="text.secondary">
      No test attempt selected, or no summary or artifacts available.
    </Typography>
  );
}
