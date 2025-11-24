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
import { useState } from 'react';

import HelpTooltip from '@/clusters/components/help_tooltip/help_tooltip';
import { ErrorDisplay } from '@/common/components/error_handling/error_display';
import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { CompareArtifactLinesRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { useRecentPasses } from '@/test_investigation/context';
import { useFetchArtifactContentQuery } from '@/test_investigation/hooks/queries';

import { LogComparisonView } from './log_comparison_view';
import { TextBox } from './text_box';
import { TextDiffArtifactView } from './text_diff_artifact_view';

interface ArtifactContentViewProps {
  artifact: Artifact;
}

export function ArtifactContentView({ artifact }: ArtifactContentViewProps) {
  const [showRawContentView, setShowRawContentView] = useState(false);
  const [showLogComparison, setShowLogComparison] = useState(false);
  const resultDbClient = useResultDbClient();
  const { passingResults, error: recentPassesError } = useRecentPasses();
  const hasPassingResults = !!passingResults && passingResults.length > 0;

  const { data: artifactContentData, isLoading: isLoadingArtifactContent } =
    useFetchArtifactContentQuery({
      artifact: artifact,
    });

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
        </Box>
      </Box>

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
        />
      ) : artifactContentData?.error ? (
        <Typography sx={{ mt: 1, color: 'error.main' }}>
          Error loading artifact content: {artifactContentData.error.message}
        </Typography>
      ) : isTextDiff &&
        !showRawContentView &&
        artifactContentData?.isText &&
        artifactContentData.data ? (
        <TextDiffArtifactView artifact={artifact} />
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
