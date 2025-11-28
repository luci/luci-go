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
import { useQuery } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels';

import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { parseWorkUnitTestResultName } from '@/common/tools/test_result_utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { GetArtifactRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { useTestVariant } from '@/test_investigation/context';

import { ArtifactSummaryView } from './artifact_content';
import { ArtifactContentView } from './artifact_content/artifact_content_view';
import { ArtifactsTreeLayout } from './artifact_tree/artifact_tree_layout';
import { ArtifactTreeView } from './artifact_tree/artifact_tree_view';
import { ArtifactFilterProvider } from './artifact_tree/context/provider';
import { WorkUnitArtifactsTreeView } from './artifact_tree/work_unit_artifacts_tree_view';
import { ArtifactsProvider, useArtifactsContext } from './context';

interface ArtifactsExplorerProps {
  rootInvocationId?: string;
  workUnitId?: string;
  textDiffArtifact?: Artifact;
}

function ArtifactsExplorer({
  rootInvocationId,
  workUnitId,
  textDiffArtifact,
}: ArtifactsExplorerProps) {
  const { currentResult, selectedAttemptIndex, selectedArtifact } =
    useArtifactsContext();
  const [viewMode, setViewMode] = useState<'artifacts' | 'work-units'>(
    'artifacts',
  );

  if (!currentResult) {
    return null;
  }

  return (
    <ArtifactFilterProvider>
      <PanelGroup
        direction="horizontal"
        style={{ height: '100%', minHeight: '600px' }}
        // This will make the panel size stored in local storage.
        autoSaveId="artifacts-panel-group-size"
      >
        <Panel defaultSize={30} minSize={20}>
          <ArtifactsTreeLayout
            viewMode={viewMode}
            onViewModeChange={setViewMode}
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
        <Panel defaultSize={70} minSize={30}>
          <Box
            sx={{
              p: 2,
              height: '100%',
              overflowY: 'auto',
              wordBreak: 'break-all',
            }}
          >
            {selectedArtifact ? (
              selectedArtifact.isSummary ? (
                <ArtifactSummaryView
                  currentResult={currentResult}
                  textDiffArtifact={textDiffArtifact}
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
    </ArtifactFilterProvider>
  );
}

function ArtifactsSectionContent() {
  const resultDbClient = useResultDbClient();
  const { currentResult } = useArtifactsContext();

  const parsedWorkUnitName = useMemo(
    () =>
      currentResult ? parseWorkUnitTestResultName(currentResult.name) : null,
    [currentResult],
  );
  const rootInvocationId = parsedWorkUnitName?.rootInvocationId;
  const workUnitId = parsedWorkUnitName?.workUnitId;

  const { data: textDiffArtifact } = useQuery({
    ...resultDbClient.GetArtifact.query(
      GetArtifactRequest.fromPartial({
        name: `${currentResult?.name}/artifacts/text_diff`,
      }),
    ),
    enabled: !!currentResult?.name,
    staleTime: Infinity,
    retry: false,
  });

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
          textDiffArtifact={textDiffArtifact}
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
