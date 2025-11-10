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

import SearchIcon from '@mui/icons-material/Search';
import TuneIcon from '@mui/icons-material/Tune';
import {
  Box,
  Chip,
  ClickAwayListener,
  IconButton,
  InputAdornment,
  LinearProgress,
  TextField,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useMemo, useRef } from 'react';

import {
  TreeData,
  VirtualTree,
  VirtualTreeNodeActions,
} from '@/common/components/log_viewer';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';

import { ClusteringControls } from '../clustering_controls';
import { useArtifactsContext } from '../context';
import { ArtifactTreeNodeData } from '../types';

import { ArtifactFiltersDropdown } from './artifact_filters_dropdown';
import { ArtifactTreeNode } from './artifact_tree_node';
import { ArtifactFilterProvider, useArtifactFilters } from './context/';

interface ArtifactTreeViewInternalProps {
  artifactsLoading: boolean;
  selectedArtifact?: ArtifactTreeNodeData | null;
  updateSelectedArtifact: (artifact: ArtifactTreeNodeData | null) => void;
}

function ArtifactTreeViewInternal({
  artifactsLoading,
  selectedArtifact: selectedArtifactNode,
  updateSelectedArtifact,
}: ArtifactTreeViewInternalProps) {
  const { clusteredFailures, hasRenderableResults } = useArtifactsContext();
  const {
    searchTerm,
    setSearchTerm,
    debouncedSearchTerm,
    finalArtifactsTree,
    isFilterPanelOpen,
    setIsFilterPanelOpen,
  } = useArtifactFilters();

  const filterContainerRef = useRef<HTMLDivElement>(null);

  const noFailuresToClusterMessage = 'No failures to cluster.';

  useEffect(() => {
    if (debouncedSearchTerm) return;
    if (selectedArtifactNode) return;

    const summaryNode = finalArtifactsTree.find((node) => node.isSummary);
    if (summaryNode) {
      updateSelectedArtifact(summaryNode);
      return;
    }

    const findFirstLeafRecursive = (
      nodes: readonly ArtifactTreeNodeData[],
    ): ArtifactTreeNodeData | null => {
      for (const node of nodes) {
        if (!node.children || node.children.length === 0) {
          if (node.artifact || node.isSummary) return node;
        }
        if (node.children) {
          const found = findFirstLeafRecursive(node.children);
          if (found) return found;
        }
      }
      return null;
    };

    const firstLeaf = findFirstLeafRecursive(finalArtifactsTree);
    updateSelectedArtifact(firstLeaf);
  }, [
    finalArtifactsTree,
    updateSelectedArtifact,
    selectedArtifactNode,
    debouncedSearchTerm,
  ]);

  const setActiveSelectionFnForSelectedNode = useCallback(
    (nodeData: ArtifactTreeNodeData): boolean => {
      if (!selectedArtifactNode) return false;
      if (selectedArtifactNode.isSummary) return !!nodeData.isSummary;
      return (
        nodeData.artifact?.artifactId ===
        selectedArtifactNode.artifact?.artifactId
      );
    },
    [selectedArtifactNode],
  );

  const selectedNodes: Set<string> | undefined = useMemo(() => {
    if (selectedArtifactNode) {
      return new Set([selectedArtifactNode.id]);
    }
    return undefined;
  }, [selectedArtifactNode]);

  const selectedArtifactLabel = useMemo(() => {
    if (selectedArtifactNode) {
      if (selectedArtifactNode.isSummary) {
        return 'Summary';
      } else if (selectedArtifactNode.artifact) {
        return selectedArtifactNode.artifact.artifactId;
      }
    }
    return '';
  }, [selectedArtifactNode]);

  if (artifactsLoading) {
    return <LinearProgress />;
  }

  function handleLeafNodeClicked(node: ArtifactTreeNodeData) {
    if (node.artifact || node.isSummary) {
      updateSelectedArtifact(node);
    }
  }

  function handleUnsupportedLeafNodeClicked(nodeData: ArtifactTreeNodeData) {
    open(
      getRawArtifactURLPath(nodeData.artifact?.name || nodeData.url || ''),
      '_blank',
    );
  }

  return (
    <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <Box
        sx={{ p: 1, pb: 0, display: 'flex', flexDirection: 'column', gap: 2 }}
      >
        {clusteredFailures.length > 0 ? (
          <ClusteringControls />
        ) : (
          hasRenderableResults && (
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              {noFailuresToClusterMessage}
            </Typography>
          )
        )}
        <ClickAwayListener onClickAway={() => setIsFilterPanelOpen(false)}>
          <Box ref={filterContainerRef} sx={{ position: 'relative' }}>
            <TextField
              placeholder="Search for artifact"
              variant="outlined"
              size="small"
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              fullWidth
              slotProps={{
                input: {
                  startAdornment: (
                    <InputAdornment position="start">
                      <SearchIcon />
                    </InputAdornment>
                  ),
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        onClick={() => setIsFilterPanelOpen((prev) => !prev)}
                        sx={{
                          color: isFilterPanelOpen ? 'primary.main' : 'inherit',
                        }}
                      >
                        <TuneIcon />
                      </IconButton>
                    </InputAdornment>
                  ),
                },
              }}
              sx={{
                '& .MuiOutlinedInput-root': {
                  borderRadius: '50px',
                  backgroundColor: 'action.hover',
                  '& fieldset': {
                    border: 'none',
                  },
                },
              }}
            />
            {isFilterPanelOpen && (
              <Box
                sx={{
                  position: 'absolute',
                  top: 'calc(100% + 4px)',
                  left: 0,
                  width: '100%',
                  zIndex: (theme) => theme.zIndex.tooltip,
                }}
              >
                <ArtifactFiltersDropdown />
              </Box>
            )}
          </Box>
        </ClickAwayListener>
        {selectedArtifactNode && (
          <Box>
            Selected artifact:{' '}
            <Chip size="small" label={selectedArtifactLabel} sx={{ ml: 1 }} />
          </Box>
        )}
      </Box>
      <Box
        sx={{
          flexGrow: 1,
          width: '100%',
          wordBreak: 'break-word',
          overflowY: 'auto',
        }}
      >
        <VirtualTree<ArtifactTreeNodeData>
          root={finalArtifactsTree}
          isTreeCollapsed={false}
          scrollToggle
          itemContent={(
            index: number,
            row: TreeData<ArtifactTreeNodeData>,
            context: VirtualTreeNodeActions<ArtifactTreeNodeData>,
          ) => (
            <ArtifactTreeNode
              index={index}
              row={row}
              context={context}
              onSupportedLeafClick={handleLeafNodeClicked}
              onUnsupportedLeafClick={handleUnsupportedLeafNodeClicked}
              highlightText={debouncedSearchTerm}
            />
          )}
          selectedNodes={selectedNodes}
          setActiveSelectionFn={setActiveSelectionFnForSelectedNode}
        />
      </Box>
    </Box>
  );
}

interface ArtifactTreeViewProps {
  resultArtifacts: readonly Artifact[];
  invArtifacts: readonly Artifact[];
  artifactsLoading: boolean;
  selectedArtifact?: ArtifactTreeNodeData | null;
  updateSelectedArtifact: (artifact: ArtifactTreeNodeData | null) => void;
}

export function ArtifactTreeView({
  resultArtifacts,
  invArtifacts,
  artifactsLoading,
  selectedArtifact,
  updateSelectedArtifact,
}: ArtifactTreeViewProps) {
  return (
    <ArtifactFilterProvider
      resultArtifacts={resultArtifacts}
      invArtifacts={invArtifacts}
    >
      <ArtifactTreeViewInternal
        artifactsLoading={artifactsLoading}
        selectedArtifact={selectedArtifact}
        updateSelectedArtifact={updateSelectedArtifact}
      />
    </ArtifactFilterProvider>
  );
}
