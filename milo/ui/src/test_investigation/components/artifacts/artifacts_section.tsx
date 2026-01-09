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
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels';

import { parseWorkUnitTestResultName } from '@/common/tools/test_result_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { useTestVariant } from '@/test_investigation/context';

import { ArtifactContentView } from '../common/artifacts/content/artifact_content_view';
import { ArtifactsTreeLayout } from '../common/artifacts/tree/artifact_tree_layout';
import { ArtifactTreeView } from '../common/artifacts/tree/artifact_tree_view/artifact_tree_view';
import { ArtifactFilterProvider } from '../common/artifacts/tree/context/provider';

import { ArtifactSummaryView } from './artifact_summary/artifact_summary_view';
import { ArtifactsLoader } from './artifacts_loader';
import { ClusteringControls } from './clustering_controls';
import { ArtifactsProvider, useArtifactsContext } from './context';
import { WorkUnitArtifactsTreeView } from './work_unit_tree/work_unit_artifacts_tree_view';

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

  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const viewMode =
    (searchParams.get('view') as ArtifactsViewMode) || 'artifacts';
  const setViewMode = (mode: ArtifactsViewMode) => {
    setSearchParams(
      (params) => {
        params.set('view', mode);
        return params;
      },
      { replace: true },
    );
  };

  if (!currentResult) {
    return null;
  }

  return (
    <ArtifactFilterProvider>
      <ArtifactsLoader>
        <PanelGroup
          direction="horizontal"
          // overflow: visible here and below is required to enable sticky headers within the panels.
          style={{
            overflowY: 'visible',
            overflowX: 'clip',
            minHeight: '60vh',
          }}
          // This will make the panel size stored in local storage.
          autoSaveId="artifacts-panel-group-size"
        >
          <Panel
            defaultSize={30}
            minSize={20}
            style={{ overflowY: 'visible', overflowX: 'clip' }}
          >
            <ArtifactsTreeLayout
              viewMode={viewMode}
              onViewModeChange={setViewMode}
              headerControls={
                hasRenderableResults && clusteredFailures ? (
                  <ClusteringControls />
                ) : null
              }
            >
              {viewMode === 'artifacts' ? (
                <ArtifactTreeView />
              ) : (
                <WorkUnitArtifactsTreeView
                  rootInvocationId={rootInvocationId || ''}
                  workUnitId={workUnitId || ''}
                />
              )}
            </ArtifactsTreeLayout>
          </Panel>
          <PanelResizeHandle>
            <Box
              sx={{
                width: '8px',
                height: '100%',
                cursor: 'col-resize',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                bgcolor: 'action.hover',
                '&:hover': { bgcolor: 'action.selected' },
              }}
            >
              <Box sx={{ width: '2px', height: '24px', bgcolor: 'divider' }} />
            </Box>
          </PanelResizeHandle>
          <Panel
            defaultSize={70}
            minSize={30}
            style={{ overflowY: 'visible', overflowX: 'clip', minWidth: 0 }}
          >
            <Box
              sx={{
                width: '100%',
              }}
            >
              {selectedArtifact ? (
                selectedArtifact.isSummary ? (
                  <ArtifactSummaryView
                    currentResult={currentResult}
                    selectedAttemptIndex={selectedAttemptIndex}
                  />
                ) : selectedArtifact.artifact ? (
                  <ArtifactContentView artifact={selectedArtifact.artifact} />
                ) : (
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      height: '100%',
                      color: 'text.secondary',
                    }}
                  >
                    <Typography>Select an artifact to view.</Typography>
                  </Box>
                )
              ) : (
                <Box
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    height: '100%',
                    color: 'text.secondary',
                  }}
                >
                  <Typography>Select an artifact to view.</Typography>
                </Box>
              )}
            </Box>
          </Panel>
        </PanelGroup>
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
