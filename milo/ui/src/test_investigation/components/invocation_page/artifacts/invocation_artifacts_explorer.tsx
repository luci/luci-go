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
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels';

import { ArtifactContentView } from '@/test_investigation/components/common/artifacts/content/artifact_content_view';
import { useArtifacts } from '@/test_investigation/components/common/artifacts/context/context';
import { ArtifactsTreeLayout } from '@/test_investigation/components/common/artifacts/tree/artifact_tree_layout';
import { ArtifactTreeView } from '@/test_investigation/components/common/artifacts/tree/artifact_tree_view/artifact_tree_view';
import { ArtifactFilterProvider } from '@/test_investigation/components/common/artifacts/tree/context/provider';
import { InvocationArtifactSummary } from '@/test_investigation/components/invocation_page/artifacts/invocation_artifact_summary';

import { InvocationArtifactsLoader } from './invocation_artifacts_loader';

export function InvocationArtifactsExplorer() {
  return (
    <ArtifactFilterProvider>
      <InvocationArtifactsLoader>
        <ExplorerContent />
      </InvocationArtifactsLoader>
    </ArtifactFilterProvider>
  );
}

function ExplorerContent() {
  const { selectedNode, nodes } = useArtifacts();
  const viewMode = 'artifacts';

  if (nodes.length === 0) {
    return (
      <Box
        sx={{
          p: 2,
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          height: '100%',
        }}
      >
        <Typography color="text.secondary">No artifacts to display.</Typography>
      </Box>
    );
  }

  return (
    <PanelGroup
      direction="horizontal"
      style={{
        height: '100%',
        overflowY: 'visible',
        overflowX: 'clip',
      }}
      autoSaveId="invocation-artifacts-panel-group-size"
    >
      <Panel
        defaultSize={30}
        minSize={20}
        style={{ overflowY: 'visible', overflowX: 'clip' }}
      >
        <ArtifactsTreeLayout
          viewMode={viewMode}
          onViewModeChange={() => {}}
          hideViewModeToggle
        >
          <ArtifactTreeView />
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
        <Box sx={{ width: '100%', height: '100%', overflow: 'auto' }}>
          {selectedNode?.isSummary ? (
            <InvocationArtifactSummary />
          ) : selectedNode?.artifact ? (
            <ArtifactContentView artifact={selectedNode.artifact} />
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
  );
}
