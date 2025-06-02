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

import { Box, Chip, LinearProgress } from '@mui/material';
import { useCallback, useEffect, useMemo } from 'react';

import {
  TreeData,
  VirtualTree,
  VirtualTreeNodeActions,
} from '@/common/components/log_viewer';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

import { ArtifactTreeNode } from './artifact_tree_node';
import { ArtifactTreeNodeData, SelectedArtifactSource } from './types';

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
  containsSummary: boolean,
): ArtifactTreeNodeData[] {
  const result: ArtifactTreeNodeData[] = [];

  if (containsSummary) {
    result.push({
      id: 'summary_node',
      name: 'Summary',
      isSummary: true,
      children: [],
    });
  }
  // Inserted ids are used by the tree to identify which
  // nodes to expand and collapse.
  // This variable is used to track the ids of non leaf items.
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
  currentResult?: TestResult;
}

export function ArtifactTreeView({
  resultArtifacts,
  invArtifacts,
  artifactsLoading,
  selectedArtifact: selectedArtifactNode,
  updateSelectedArtifact,
  currentResult,
}: ArtifactTreeViewProps) {
  const artifactsTree = useMemo(() => {
    return buildArtifactsTree(
      resultArtifacts,
      invArtifacts,
      !!currentResult?.summaryHtml,
    );
  }, [resultArtifacts, invArtifacts, currentResult?.summaryHtml]);

  const setActiveSelectionFnForSelectedNode = useCallback(
    (nodeData: ArtifactTreeNodeData): boolean => {
      return !!(
        selectedArtifactNode &&
        (selectedArtifactNode.isSummary ||
          nodeData.artifact?.artifactId ===
            selectedArtifactNode.artifact?.artifactId)
      );
    },
    [selectedArtifactNode],
  );

  useEffect(() => {
    const currentNodes = artifactsTree;
    const summaryNode = currentNodes.find((node) => node.isSummary);
    if (!selectedArtifactNode) {
      if (summaryNode) {
        updateSelectedArtifact(summaryNode);
      } else if (currentNodes.length > 0) {
        const findFirstLeafRecursive = (
          nodes: ArtifactTreeNodeData[],
        ): ArtifactTreeNodeData | null => {
          for (const node of nodes) {
            if (node.children.length === 0) return node;
            if (node.children) {
              const found = findFirstLeafRecursive(node.children);
              if (found) return found;
            }
          }
          for (const node of nodes) {
            if (node.artifact) {
              return node;
            }
          }
          return null;
        };
        const firstLeaf = findFirstLeafRecursive(currentNodes);
        updateSelectedArtifact(firstLeaf);
      } else {
        updateSelectedArtifact(null);
      }
    }
  }, [artifactsTree, updateSelectedArtifact, selectedArtifactNode]);

  const selectedNodes: Set<string> | undefined = useMemo(() => {
    if (selectedArtifactNode) {
      if (selectedArtifactNode.isSummary) {
        return new Set([selectedArtifactNode.id]);
      } else if (selectedArtifactNode.artifact) {
        return new Set([selectedArtifactNode.artifact?.artifactId]);
      }
    }
    return undefined;
  }, [selectedArtifactNode]);

  const selectedArtifactLabel = useMemo(() => {
    if (selectedArtifactNode)
      if (selectedArtifactNode.isSummary) {
        return 'Summary';
      } else if (selectedArtifactNode.artifact) {
        return selectedArtifactNode.artifact.artifactId;
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
      {selectedArtifactNode && (
        <Box sx={{ p: 1 }}>
          Selected artifact:
          <Chip size="small" label={selectedArtifactLabel} />
        </Box>
      )}
      <Box sx={{ flexGrow: 1, width: '100%', wordBreak: 'break-word' }}>
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
            />
          )}
          selectedNodes={selectedNodes}
          setActiveSelectionFn={setActiveSelectionFnForSelectedNode}
        />
      </Box>
    </Box>
  );
}
