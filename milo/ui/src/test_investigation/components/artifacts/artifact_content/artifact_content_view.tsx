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

import AdbIcon from '@mui/icons-material/Adb';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import {
  Box,
  Button,
  CircularProgress,
  FormControlLabel,
  Switch,
  Typography,
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useMemo, useState } from 'react';

import HelpTooltip from '@/clusters/components/help_tooltip/help_tooltip';
import { ErrorDisplay } from '@/common/components/error_handling/error_display';
import { useResultDbClient } from '@/common/hooks/prpc_clients';
import {
  getAndroidBugToolLink,
  getRawArtifactURLPath,
} from '@/common/tools/url_utils';
import { Sticky } from '@/generic_libs/components/queued_sticky';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { CompareArtifactLinesRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { useInvocation } from '@/test_investigation/context';
import { useRecentPasses } from '@/test_investigation/context/context';
import { useFetchArtifactContentQuery } from '@/test_investigation/hooks/queries';
import { isAnTSInvocation } from '@/test_investigation/utils/test_info_utils';

import { LogComparisonView } from './log_comparison_view';
import { TextBox } from './text_box';
import { TextDiffArtifactView } from './text_diff_artifact_view';

interface ArtifactContentViewProps {
  artifact: Artifact;
}

const INITIAL_CONTENT_LIMIT = 5000;

export function ArtifactContentView({ artifact }: ArtifactContentViewProps) {
  const [showRawContentView, setShowRawContentView] = useState(false);
  const [showLogComparison, setShowLogComparison] = useState(false);
  const resultDbClient = useResultDbClient();
  const { passingResults, error: recentPassesError } = useRecentPasses();
  const hasPassingResults = !!passingResults && passingResults.length > 0;
  const invocation = useInvocation();
  const isAnTS = isAnTSInvocation(invocation);

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

  const {
    data: lineComparison,
    isLoading: isComparisonLoading,
    error: comparisonError,
    refetch: refetchComparison,
  } = useQuery({
    ...resultDbClient.CompareArtifactLines.query(
      CompareArtifactLinesRequest.fromPartial({
        name: artifact.name,
        passingResults: (passingResults || []).map((p) => p.name),
      }),
    ),
    enabled: showLogComparison && hasPassingResults,
    staleTime: Infinity,
  });

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

  const isLogComparisonPossible =
    artifactContentData?.isText && artifactContentData.data;

  // Reset the raw text view toggle if the selected artifact changes
  // or if it's no longer a text_diff artifact (by name or content type).
  if (!isTextDiff && showRawContentView) {
    setShowRawContentView(false);
  }
  const artifactDisplayName = artifact.name.slice(
    artifact.name.lastIndexOf('/') + 1,
  );
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
            {artifactDisplayName}
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
          {isLogComparisonPossible && (
            <FormControlLabel
              control={
                <Switch
                  checked={showLogComparison}
                  onChange={() => setShowLogComparison(!showLogComparison)}
                  size="small"
                />
              }
              disabled={!hasPassingResults}
              label={
                <>
                  Log Comparison
                  <HelpTooltip
                    content={
                      <>
                        {recentPassesError ? (
                          <>
                            <Typography
                              sx={{ fontWeight: 'bold', color: 'error.light' }}
                            >
                              Failed to find recent passes.
                            </Typography>
                            <Typography variant="caption">
                              {recentPassesError.message}
                            </Typography>
                            <br />
                            <br />
                          </>
                        ) : (
                          !hasPassingResults && (
                            <>
                              <Typography sx={{ fontWeight: 'bold' }}>
                                Log comparison is not possible as no passing
                                results have been found.
                              </Typography>
                              <br />
                              <br />
                            </>
                          )
                        )}
                        Log Comparison compares the log from this failure with
                        the same log file from up to five recent passes.
                        <br />
                        <br />
                        Each line is stripped of numbers, dates and other
                        commonly changing text and then checked for an exact
                        string match against each line in the passing logs. Any
                        lines that match a line in a passing log are hidden by
                        default.
                      </>
                    }
                  />
                </>
              }
            />
          )}
          {artifact.fetchUrl && (
            <Button
              variant="outlined"
              size="small"
              href={getRawArtifactURLPath(artifact.name)}
              target="_blank"
              rel="noopener noreferrer"
              endIcon={<OpenInNewIcon />}
            >
              Open Raw
            </Button>
          )}
          {isAnTS && (
            <Button
              variant="outlined"
              size="small"
              href={getAndroidBugToolLink(artifact.artifactId, invocation)}
              target="_blank"
              rel="noopener noreferrer"
              endIcon={<AdbIcon />}
            >
              Open in ABT
            </Button>
          )}
        </Box>
      </Box>
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

      {showLogComparison && isComparisonLoading ? (
        <Box sx={{ display: 'flex', alignItems: 'center', mt: 2 }}>
          <CircularProgress size={20} />
          <Typography sx={{ ml: 1 }} color="text.secondary">
            Comparing with recent passes...
          </Typography>
        </Box>
      ) : showLogComparison && comparisonError ? (
        <Box sx={{ mt: 2 }}>
          <ErrorDisplay
            error={comparisonError}
            instruction="Failed to perform log comparison."
            onTryAgain={() => refetchComparison()}
          />
        </Box>
      ) : isLogComparisonPossible &&
        lineComparison?.failureOnlyRanges &&
        showLogComparison ? (
        <LogComparisonView
          logContent={artifactContentData.data || ''}
          failureOnlyRanges={lineComparison.failureOnlyRanges}
          isFullLoading={isFullLoading}
        />
      ) : artifactContentData?.error ? (
        <Typography sx={{ mt: 1, color: 'error.main' }}>
          Error loading artifact content: {artifactContentData.error.message}
        </Typography>
      ) : isTextDiff &&
        !showRawContentView &&
        artifactContentData?.isText &&
        artifactContentData.data ? (
        <TextDiffArtifactView
          artifact={artifact}
          content={artifactContentData.data}
        />
      ) : artifactContentData?.isText ? (
        artifactContentData.data !== null && artifactContentData.data !== '' ? (
          <Box
            sx={{
              border: 'solid 1px #aaa',
              borderRadius: '3px',
              padding: '8px 0',
            }}
          >
            <TextBox lines={artifactContentData.data.split('\n')}></TextBox>
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
