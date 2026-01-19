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

import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';

import { ArtifactTreeNodeData, SelectedArtifactSource } from '../../types';

export interface ArtifactFilterOptions {
  searchTerm: string;
  artifactTypes: string[];
}

export function filterArtifacts(
  artifacts: readonly Artifact[],
  { searchTerm, artifactTypes }: ArtifactFilterOptions,
): readonly Artifact[] {
  let filtered = artifacts;

  if (searchTerm) {
    const lowerTerm = searchTerm.toLowerCase();
    filtered = filtered.filter((artifact) =>
      artifact.artifactId.toLowerCase().includes(lowerTerm),
    );
  }

  if (artifactTypes.length > 0) {
    const selectedTypes = new Set(artifactTypes);
    filtered = filtered.filter((artifact) =>
      selectedTypes.has(artifact.artifactType),
    );
  }

  return filtered;
}

export function addArtifactsToTree(
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

export function buildArtifactsTree(
  resultArtifacts: readonly Artifact[],
  invocationArtifacts: readonly Artifact[],
  options: { includeSummary?: boolean } = {},
): ArtifactTreeNodeData[] {
  const result: ArtifactTreeNodeData[] = [];

  if (options.includeSummary ?? true) {
    result.push({
      id: 'summary_node',
      name: 'Summary',
      isSummary: true,
      children: [],
    });
  }
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

export function pruneEmptyFolders(
  nodes: ArtifactTreeNodeData[],
): ArtifactTreeNodeData[] {
  const prunedNodes: ArtifactTreeNodeData[] = [];
  for (const node of nodes) {
    const isLeaf = !!node.artifact || node.isSummary;
    if (isLeaf) {
      prunedNodes.push(node);
      continue;
    }

    const prunedChildren = pruneEmptyFolders(node.children);

    if (prunedChildren.length > 0) {
      prunedNodes.push({ ...node, children: prunedChildren });
    }
  }
  return prunedNodes;
}

export function findFirstLeafRecursive(
  nodes: readonly ArtifactTreeNodeData[],
): ArtifactTreeNodeData | null {
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
}

export function findNode(
  nodes: readonly ArtifactTreeNodeData[],
  id: string,
): ArtifactTreeNodeData | null {
  for (const node of nodes) {
    if (node.id === id) {
      return node;
    }
    if (node.children) {
      const found = findNode(node.children, id);
      if (found) {
        return found;
      }
    }
  }
  return null;
}
/**
 * Map to find the display name given a WorkunitFolder type
 */
export const typeDisplayNameMap = new Map([
  ['ATP_INVOCATION', 'ATP'],
  ['TFC_COMMAND', 'TFC COMMAND'],
  ['TFC_COMMAND_TASK', 'TRADEFED'],
  ['TF_INVOCATION', 'TRADEFED'],
  ['TF_MODULE', 'MODULE'],
  ['TF_TEST_RUN', 'TEST RUN'],
]);

export function getWorkUnitLabel(
  wu: {
    kind?: string;
    workUnitId: string;
    moduleId?: { moduleName: string; moduleScheme?: string };
    moduleShardKey?: string;
  },
  rootInvocationName?: string,
): string {
  if (wu.moduleId) {
    return `Module: ${wu.moduleId.moduleName}${wu.moduleShardKey ? ` - ${wu.moduleShardKey}` : ''}`;
  }
  if (wu.workUnitId === 'root' && rootInvocationName) {
    return rootInvocationName;
  }
  if (wu.kind && typeDisplayNameMap.has(wu.kind)) {
    return typeDisplayNameMap.get(wu.kind)!;
  }
  return wu.kind || wu.workUnitId;
}
