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
import { useMemo } from 'react';

import { Sticky } from '@/generic_libs/components/queued_sticky';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { useFetchArtifactContentQuery } from '@/test_investigation/hooks/queries';

import { DefaultArtifactContent } from './default';
import { LogArtifactContent } from './logs';
import { TextDiffArtifactContent } from './text_diff';

interface ArtifactContentViewProps {
  artifact: Artifact;
}

const INITIAL_CONTENT_LIMIT = 5000;

export function ArtifactContentView({ artifact }: ArtifactContentViewProps) {
  const { data: initialContentData, isLoading: isInitialLoading } =
    useFetchArtifactContentQuery({
      artifact: artifact,
      limit: INITIAL_CONTENT_LIMIT,
    });

  const sizeBytes = artifact.sizeBytes ? Number(artifact.sizeBytes) : null;
  const isLargeArtifact =
    sizeBytes === null || sizeBytes > INITIAL_CONTENT_LIMIT;

  const { data: fullContentData, isLoading: isFullLoading } =
    useFetchArtifactContentQuery({
      artifact: artifact,
      enabled: isLargeArtifact,
    });

  const artifactContentData = useMemo(() => {
    if (!isLargeArtifact) {
      return initialContentData;
    }
    return fullContentData || initialContentData;
  }, [isLargeArtifact, initialContentData, fullContentData]);

  const isLoadingArtifactContent = isInitialLoading;

  if (isLoadingArtifactContent) {
    return (
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          height: '100px',
        }}
      >
        <CircularProgress />
      </Box>
    );
  }

  const isTextDiff =
    artifact.artifactId === 'text_diff' ||
    artifact.contentType === 'text/x-diff';

  const isLogComparisonPossible = !!(
    artifactContentData?.isText && artifactContentData.data
  );

  const displayContentType =
    artifactContentData?.contentType || artifact.contentType || 'N/A';

  return (
    <Box
      sx={{
        minWidth: 0,
      }}
    >
      {isLargeArtifact && isFullLoading && (
        <Sticky
          top
          sx={{
            zIndex: 100,
            backgroundColor: 'warning.light',
            color: 'warning.contrastText',
            p: 1,
            mb: 1,
            borderRadius: 1,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            boxShadow: 2,
            mx: 2,
          }}
        >
          <CircularProgress size={20} color="inherit" sx={{ mr: 2 }} />
          <Typography variant="body2">
            Displaying preview. Still loading full{' '}
            {sizeBytes ? (sizeBytes / 1024 / 1024).toFixed(2) : 'unknown size'}{' '}
            MB log...
          </Typography>
        </Sticky>
      )}
      {artifactContentData?.error ? (
        <Typography sx={{ mt: 1, color: 'error.main', px: 2 }}>
          Error loading artifact content: {artifactContentData.error.message}
        </Typography>
      ) : isTextDiff &&
        artifactContentData?.isText &&
        artifactContentData.data ? (
        <TextDiffArtifactContent
          artifact={artifact}
          content={artifactContentData.data}
          isFullLoading={isFullLoading}
        />
      ) : isLogComparisonPossible ? (
        <LogArtifactContent
          artifact={artifact}
          content={artifactContentData.data || ''}
          isFullLoading={isFullLoading}
        />
      ) : (
        <DefaultArtifactContent
          artifact={artifact}
          contentType={displayContentType}
        />
      )}
    </Box>
  );
}
