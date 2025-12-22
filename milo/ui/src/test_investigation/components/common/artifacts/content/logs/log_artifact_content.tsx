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
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';

import { ErrorDisplay } from '@/common/components/error_handling/error_display';
import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { CompareArtifactLinesRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { useRecentPasses } from '@/test_investigation/context/context';

import { VirtualizedArtifactContent } from '../virtualized';

interface LogArtifactContentProps {
  artifact: Artifact;
  content: string;
  isFullLoading?: boolean;
}

export function LogArtifactContent({
  artifact,
  content,
  isFullLoading,
}: LogArtifactContentProps) {
  const [showLogComparison, setShowLogComparison] = useState(false);
  const resultDbClient = useResultDbClient();
  const { passingResults } = useRecentPasses();
  const hasPassingResults = !!passingResults && passingResults.length > 0;

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

  return (
    <>
      {/*
        We rely on VirtualizedArtifactContent to render the header because it manages
        search query state and other internal UI that lives in the header.
        We just pass our actions (Log Comparison toggle, Open Raw, Open in ABT) to it.
      */}
      {showLogComparison && isComparisonLoading ? (
        <Box sx={{ display: 'flex', alignItems: 'center', mt: 2, px: 2 }}>
          <CircularProgress size={20} />
          <Typography sx={{ ml: 1 }} color="text.secondary">
            Comparing with recent passes...
          </Typography>
        </Box>
      ) : showLogComparison && comparisonError ? (
        <Box sx={{ mt: 2, px: 2 }}>
          <ErrorDisplay
            error={comparisonError}
            instruction="Failed to perform log comparison."
            onTryAgain={() => refetchComparison()}
          />
        </Box>
      ) : (
        <VirtualizedArtifactContent
          content={content}
          failureOnlyRanges={
            showLogComparison ? lineComparison?.failureOnlyRanges : undefined
          }
          isFullLoading={isFullLoading}
          artifact={artifact}
          hasPassingResults={hasPassingResults}
          isLogComparisonPossible={true}
          showLogComparison={showLogComparison}
          onToggleLogComparison={() => setShowLogComparison(!showLogComparison)}
        />
      )}
    </>
  );
}
