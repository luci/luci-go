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
import {
  Box,
  Chip,
  InputAdornment,
  LinearProgress,
  TextField,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';

import {
  TreeData,
  VirtualTree,
  VirtualTreeNodeActions,
} from '@/common/components/log_viewer';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';

import { ClusteringControls } from '../clustering_controls';
import { useArtifactsContext } from '../context';
import { ArtifactTreeNodeData, SelectedArtifactSource } from '../types';

import { ArtifactTreeNode } from './artifact_tree_node';

function addArtifactsToTree(
  artifacts: readonly Artifact[],
  root: ArtifactTreeNodeData,
  idCounter: number,
  source: SelectedArtifactSource,
) {
  for (const artifact of artifacts) {
    const path = artifact.artifactId;
    const parts = path.split('/');
    let currentNode = root;

    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      if (part) {
        let childNode: ArtifactTreeNodeData | undefined =
          currentNode.children.find((c) => c.name === part);

        if (!childNode) {
          childNode = { id: `${idCounter++}`, name: part, children: [] };
          currentNode.children.push(childNode);
        }
        currentNode = childNode as ArtifactTreeNodeData;
      }
    }
    currentNode.viewingSupported = artifact.hasLines;
    currentNode.size = Number(artifact.sizeBytes);
    currentNode.url = getRawArtifactURLPath(artifact.name);
    currentNode.artifact = artifact;
    currentNode.source = source;
    currentNode.id = artifact.artifactId;
  }
  return idCounter;
}

function buildArtifactsTree(
  resultArtifacts: readonly Artifact[],
  invocationArtifacts: readonly Artifact[],
): ArtifactTreeNodeData[] {
  const result: ArtifactTreeNodeData[] = [];

  result.push({
    id: 'summary_node',
    name: 'Summary',
    isSummary: true,
    children: [],
  });
  let lastInsertedId = 0;

  if (resultArtifacts.length > 0) {
    const resultArtifactsRoot: ArtifactTreeNodeData = {
      id: `${++lastInsertedId}`,
      name: 'Result artifacts',
      children: [],
    };
    lastInsertedId = addArtifactsToTree(
      resultArtifacts,
      resultArtifactsRoot,
      ++lastInsertedId,
      'result',
    );

    result.push(resultArtifactsRoot);
  }

  if (invocationArtifacts.length > 0) {
    const invocationArtifactsRoot: ArtifactTreeNodeData = {
      id: `${++lastInsertedId}`,
      name: 'Invocation artifacts',
      children: [],
    };
    addArtifactsToTree(
      invocationArtifacts,
      invocationArtifactsRoot,
      ++lastInsertedId,
      'invocation',
    );
    result.push(invocationArtifactsRoot);
  }
  return result;
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
  selectedArtifact: selectedArtifactNode,
  updateSelectedArtifact,
}: ArtifactTreeViewProps) {
  const { clusteredFailures, hasRenderableResults } = useArtifactsContext();
  const [searchTerm, setSearchTerm] = useState('');
  const [debouncedSearchTerm, setDebouncedSearchTerm] = useState('');

  const noFailuresToClusterMessage = 'No failures to cluster.';

  useEffect(() => {
    const timerId = setTimeout(() => {
      setDebouncedSearchTerm(searchTerm);
    }, 300);

    return () => {
      clearTimeout(timerId);
    };
  }, [searchTerm]);

  const filteredResultArtifacts = useMemo(() => {
    if (!debouncedSearchTerm) {
      return resultArtifacts;
    }
    return resultArtifacts.filter((artifact) =>
      artifact.artifactId
        .toLowerCase()
        .includes(debouncedSearchTerm.toLocaleLowerCase()),
    );
  }, [resultArtifacts, debouncedSearchTerm]);

  const filteredInvArtifacts = useMemo(() => {
    if (!debouncedSearchTerm) {
      return invArtifacts;
    }
    return invArtifacts.filter((artifact) =>
      artifact.artifactId
        .toLowerCase()
        .includes(debouncedSearchTerm.toLocaleLowerCase()),
    );
  }, [invArtifacts, debouncedSearchTerm]);

  const artifactsTree = useMemo(() => {
    return buildArtifactsTree(filteredResultArtifacts, filteredInvArtifacts);
  }, [filteredResultArtifacts, filteredInvArtifacts]);

  useEffect(() => {
    if (debouncedSearchTerm) return;
    if (selectedArtifactNode) return;

    const summaryNode = artifactsTree.find((node) => node.isSummary);
    if (summaryNode) {
      updateSelectedArtifact(summaryNode);
      return;
    }

    const findFirstLeafRecursive = (
      nodes: ArtifactTreeNodeData[],
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

    const firstLeaf = findFirstLeafRecursive(artifactsTree);
    updateSelectedArtifact(firstLeaf);
  }, [
    artifactsTree,
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
        <TextField
          placeholder="Search artifacts"
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
            },
          }}
          sx={{
            '& .MuiOutlinedInput-root': {
              borderRadius: '50px',
              backgroundColor: '#f5f5f5',
              '& fieldset': {
                border: 'none',
              },
            },
          }}
        />
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
          root={artifactsTree}
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
