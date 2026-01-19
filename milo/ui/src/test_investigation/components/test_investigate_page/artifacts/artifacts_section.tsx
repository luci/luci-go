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
import { useMemo } from 'react';

import { parseWorkUnitTestResultName } from '@/common/tools/test_result_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { ArtifactsSplitView } from '@/test_investigation/components/common/artifacts/artifacts_split_view';
import { ArtifactContentView } from '@/test_investigation/components/common/artifacts/content/artifact_content_view';
import { ArtifactTreeView } from '@/test_investigation/components/common/artifacts/tree/artifact_tree_view/artifact_tree_view';
import { ArtifactFilterProvider } from '@/test_investigation/components/common/artifacts/tree/context/provider';
import { ArtifactSummaryView } from '@/test_investigation/components/test_investigate_page/artifacts/artifact_summary/artifact_summary_view';
import { useTestVariant } from '@/test_investigation/context';

import { ArtifactsLoader } from './artifacts_loader';
import { ClusteringControls } from './clustering_controls';
import { ArtifactsProvider, useArtifactsContext } from './context';
import { TestResultWorkUnitTreeView } from './work_unit_tree';

interface ArtifactsExplorerProps {
  rootInvocationId?: string;
  workUnitId?: string;
}

export type ArtifactsViewMode = 'artifacts' | 'work-units';

function ArtifactsExplorer({
  rootInvocationId,
  workUnitId,
}: ArtifactsExplorerProps) {
  const {
    currentResult,
    selectedAttemptIndex,
    selectedArtifact,
    hasRenderableResults,
    clusteredFailures,
  } = useArtifactsContext();

  const [searchParams] = useSyncedSearchParams();
  const viewMode =
    (searchParams.get('view') as ArtifactsViewMode) || 'artifacts';

  if (!currentResult) {
    return null;
  }

  return (
    <ArtifactFilterProvider>
      <ArtifactsLoader>
        <ArtifactsSplitView
          sidebarContent={
            viewMode === 'artifacts' ? (
              <ArtifactTreeView />
            ) : (
              <TestResultWorkUnitTreeView
                rootInvocationId={rootInvocationId || ''}
                workUnitId={workUnitId || ''}
              />
            )
          }
          selectedNode={selectedArtifact}
          headerControls={
            hasRenderableResults && clusteredFailures ? (
              <ClusteringControls />
            ) : null
          }
          renderContent={(node) => {
            if (node.isSummary) {
              return (
                <ArtifactSummaryView
                  currentResult={currentResult}
                  selectedAttemptIndex={selectedAttemptIndex}
                />
              );
            }
            if (node.artifact) {
              return <ArtifactContentView artifact={node.artifact} />;
            }
            return null;
          }}
        />
      </ArtifactsLoader>
    </ArtifactFilterProvider>
  );
}

function ArtifactsSectionContent() {
  const { currentResult } = useArtifactsContext();

  const parsedWorkUnitName = useMemo(
    () =>
      currentResult ? parseWorkUnitTestResultName(currentResult.name) : null,
    [currentResult],
  );
  const rootInvocationId = parsedWorkUnitName?.rootInvocationId;
  const workUnitId = parsedWorkUnitName?.workUnitId;

  return (
    <Box
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      {currentResult ? (
        <ArtifactsExplorer
          rootInvocationId={rootInvocationId}
          workUnitId={workUnitId}
        />
      ) : (
        <Typography color="text.disabled">
          No test results to display.
        </Typography>
      )}
    </Box>
  );
}

export function ArtifactsSection() {
  const testVariant = useTestVariant();

  if (!testVariant.results) {
    return (
      <Typography color="text.disabled">No test results to display.</Typography>
    );
  }

  return (
    <ArtifactsProvider results={testVariant.results}>
      <ArtifactsSectionContent />
    </ArtifactsProvider>
  );
}
