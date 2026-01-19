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
import { ReactNode, useCallback } from 'react';
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { ArtifactContentView } from '@/test_investigation/components/common/artifacts/content/artifact_content_view';
import { ArtifactsTreeLayout } from '@/test_investigation/components/common/artifacts/tree/artifact_tree_layout';
import { ArtifactTreeNodeData } from '@/test_investigation/components/common/artifacts/types';

export type ArtifactsViewMode = 'artifacts' | 'work-units';

interface ArtifactsSplitViewProps {
  /**
   * Component to render in the sidebar (the artifacts tree).
   */
  sidebarContent: ReactNode;
  /**
   * The currently selected node, if any.
   */
  selectedNode: ArtifactTreeNodeData | null;
  /**
   * Optional function to render custom content based on the selected node.
   * If provided, specific return values (non-null) will override the default artifact content view.
   * Return null to use the default empty state or fallback behavior.
   */
  renderContent?: (selectedNode: ArtifactTreeNodeData) => ReactNode;

  /**
   * Additional controls for the header.
   */
  headerControls?: ReactNode;

  /**
   * Whether to hide the view mode toggle.
   */
  hideViewModeToggle?: boolean;
}

export function ArtifactsSplitView({
  sidebarContent,
  selectedNode,
  renderContent,
  headerControls,
  hideViewModeToggle,
}: ArtifactsSplitViewProps) {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const viewMode =
    (searchParams.get('view') as ArtifactsViewMode) || 'artifacts';

  const setViewMode = useCallback(
    (mode: ArtifactsViewMode) => {
      setSearchParams(
        (params) => {
          params.set('view', mode);
          return params;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  return (
    <PanelGroup
      direction="horizontal"
      // overflow: visible here and below is required to enable sticky headers within the panels.
      style={{
        overflowY: 'visible',
        overflowX: 'clip',
        minHeight: '60vh',
      }}
      // This will make the panel size stored in local storage per-page context.
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
          headerControls={headerControls}
          hideViewModeToggle={hideViewModeToggle}
          selectedNodeName={selectedNode?.name}
        >
          {sidebarContent}
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
            height: '100%',
          }}
        >
          {selectedNode ? (
            renderContent ? (
              renderContent(selectedNode)
            ) : selectedNode.artifact ? (
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
  );
}
