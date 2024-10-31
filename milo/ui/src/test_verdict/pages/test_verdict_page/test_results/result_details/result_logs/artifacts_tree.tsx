// Copyright 2024 The LUCI Authors.
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

import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { Box, Chip, LinearProgress } from '@mui/material';
import { useMemo } from 'react';

import {
  LogsTreeNode,
  ObjectNode,
  VirtualTree,
} from '@/common/components/log-viewer';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';

import {
  useArtifactsLoading,
  useInvArtifacts,
  useResultArtifacts,
} from '../context';

import {
  useSelectedArtifact,
  useUpdateSelectedArtifact,
  SelectedArtifactSource,
} from './context';

interface ArtifactTreeNode extends ObjectNode {
  artifact?: Artifact;
  source?: SelectedArtifactSource;
}

function addArtifactsToTree(
  artifacts: readonly Artifact[],
  root: ArtifactTreeNode,
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
        let childNode = currentNode.children.find((c) => c.name === part);

        if (!childNode) {
          childNode = { id: idCounter++, name: part, children: [] };
          currentNode.children.push(childNode);
        }
        currentNode = childNode as ArtifactTreeNode;
      }
    }
    currentNode.viewingsupported = artifact.hasLines;
    currentNode.size = Number(artifact.sizeBytes);
    currentNode.url = getRawArtifactURLPath(artifact.name);
    currentNode.artifact = artifact;
    currentNode.source = source;
  }
  return idCounter;
}

function buildArtifactsTree(
  resultArtifacts: readonly Artifact[],
  invocationArtifacts: readonly Artifact[],
): ArtifactTreeNode {
  // The virtual tree doesn't support multiple roots so
  // we use a placeholder root so that we can split the tree
  // into result artifacts and invocation artifacts.
  const root: ArtifactTreeNode = {
    id: 0,
    name: '/',
    children: [],
  };
  // Inserted ids are used by the tree to identify which
  // nodes to expand and collapse. This variable used to track the ids.
  let lastInsertedId = 0;
  if (resultArtifacts.length > 0) {
    const resultArtifactsRoot: ArtifactTreeNode = {
      id: ++lastInsertedId,
      name: 'Result artifacts',
      children: [],
    };
    root.children.push(resultArtifactsRoot);
    lastInsertedId = addArtifactsToTree(
      resultArtifacts,
      resultArtifactsRoot,
      ++lastInsertedId,
      'result',
    );
  }

  if (invocationArtifacts.length > 0) {
    const invocationArtifactsRoot: ArtifactTreeNode = {
      id: ++lastInsertedId,
      name: 'Invocation artifacts',
      children: [],
    };
    root.children.push(invocationArtifactsRoot);
    addArtifactsToTree(
      invocationArtifacts,
      invocationArtifactsRoot,
      ++lastInsertedId,
      'invocation',
    );
  }
  return root;
}

export function ArtifactsTree() {
  const resultArtifacts = useResultArtifacts();
  const invArtifacts = useInvArtifacts();
  const artifactsLoading = useArtifactsLoading();
  const updateSelectedArtifact = useUpdateSelectedArtifact();
  const selectedArtifact = useSelectedArtifact();

  const artifactsTree = useMemo(() => {
    if (artifactsLoading) {
      return [];
    }
    return [buildArtifactsTree(resultArtifacts, invArtifacts)];
  }, [resultArtifacts, invArtifacts, artifactsLoading]);

  if (artifactsLoading) {
    return <LinearProgress />;
  }

  function handleLeafNodeClicked(node: ArtifactTreeNode) {
    if (node.artifact) {
      updateSelectedArtifact(node.artifact, node.source || null);
    }
  }

  function handleUnsupportedNodeClicked(artifactName: string) {
    open(getRawArtifactURLPath(artifactName), '_blank');
  }

  return (
    <Box
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      {selectedArtifact && (
        <Box
          sx={{
            p: 1,
          }}
        >
          Selected artifact:
          <Chip size="small" label={selectedArtifact.artifactId} />
        </Box>
      )}
      <Box
        sx={{
          flexGrow: 1,
          width: '100%',
          wordBreak: 'break-word',
        }}
      >
        <VirtualTree<ArtifactTreeNode>
          root={artifactsTree}
          isTreeCollapsed={false}
          itemContent={(index, row, context) => (
            <LogsTreeNode
              treeIndentBorder={false}
              treeFontSize="body2"
              index={index}
              treeNodeData={row}
              labels={{
                nonSupportedLeafNodeTooltip: 'Unsupported artifact type',
                specialNodeInfoTooltip: 'Special node',
              }}
              collapseIcon={<ExpandMoreIcon sx={{ fontSize: '18px' }} />}
              expandIcon={<ChevronRightIcon sx={{ fontSize: '18px' }} />}
              onNodeSelect={context.onNodeSelect!}
              onNodeToggle={context.onNodeToggle!}
              treeNodeIndentation={10}
              isSelected={context.isSelected}
              isActiveSelection={context.isActiveSelection}
              isSearchMatch={context.isSearchMatch}
              onUnsupportedLeafNodeClick={(node) => {
                // TODO: Remove this check when the bug is fixed in the common log viewer.
                if (row.isLeafNode) {
                  handleUnsupportedNodeClicked(
                    (node as ArtifactTreeNode).artifact?.name || '',
                  );
                }
              }}
              onLeafNodeClick={handleLeafNodeClicked}
            />
          )}
          selectedNodes={
            selectedArtifact
              ? new Set([selectedArtifact.artifactId])
              : undefined
          }
        />
      </Box>
    </Box>
  );
}
