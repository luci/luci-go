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
import {
  Box,
  Button,
  CircularProgress,
  FormControlLabel,
  Switch,
  Typography,
} from '@mui/material';
import { useState } from 'react';

import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

import { FetchedArtifactContent, ArtifactTreeNodeData } from '../types';

import { TextDiffArtifactView } from './text_diff_artifact_view';

interface ArtifactContentViewProps {
  selectedArtifactForDisplay: ArtifactTreeNodeData;
  currentResult?: TestResult;
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
}: ArtifactContentViewProps) {
  const [showRawContentView, setShowRawContentView] = useState(false);

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

  if (selectedArtifactForDisplay.artifact) {
    const artifact = selectedArtifactForDisplay.artifact;
    const isTextDiff =
      artifact.artifactId === 'text_diff' ||
      artifact.contentType === 'text/x-diff';

    // Reset the raw text view toggle if the selected artifact changes
    // or if it's no longer a text_diff artifact (by name or content type).
    if (!isTextDiff && showRawContentView) {
      setShowRawContentView(false);
    }
    return (
      <Box>
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            flexWrap: 'wrap',
            gap: 2,
            mb: 1,
            justifyContent: 'space-between',
          }}
        >
          <Box sx={{ display: 'flex', gap: 2, alignItems: 'center' }}>
            <Typography variant="h6" gutterBottom>
              {selectedArtifactForDisplay.name}
            </Typography>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
              <Typography variant="body2" color="text.secondary">
                Content Type:{' '}
                {artifactContentData?.contentType ||
                  artifact.contentType ||
                  'N/A'}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Size:{' '}
                {artifact.sizeBytes !== null
                  ? `${artifact.sizeBytes} bytes`
                  : 'N/A'}
              </Typography>
            </Box>
          </Box>

          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            {isTextDiff && (
              <FormControlLabel
                control={
                  <Switch
                    checked={!showRawContentView}
                    onChange={() => setShowRawContentView(!showRawContentView)}
                    size="small"
                  />
                }
                label="Formatted"
              />
            )}
            {artifact.fetchUrl && (
              <Button
                variant="outlined"
                size="small"
                href={getRawArtifactURLPath(artifact.name)}
                target="_blank"
                rel="noopener noreferrer"
                startIcon={<DescriptionIcon />}
              >
                Download
              </Button>
            )}
          </Box>
        </Box>

        {artifactContentData?.error ? (
          <Typography sx={{ mt: 1, color: 'error.main' }}>
            Error loading artifact content: {artifactContentData.error.message}
          </Typography>
        ) : isTextDiff &&
          !showRawContentView &&
          artifactContentData?.isText &&
          artifactContentData.data ? (
          <TextDiffArtifactView artifact={artifact} />
        ) : artifactContentData?.isText ? (
          artifactContentData.data !== null &&
          artifactContentData.data !== '' ? (
            <Box
              component="pre"
              sx={{
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-all',
                maxHeight: '55vh',
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
        ) : artifact.contentType ? (
          <Typography sx={{ mt: 1 }} color="text.secondary">
            Preview not available for content type:{' '}
            {artifactContentData?.contentType || artifact.contentType}.
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
