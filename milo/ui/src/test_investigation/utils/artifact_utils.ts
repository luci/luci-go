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

import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { TestResult } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

import { CustomArtifactTreeNode } from '../components/test_details/types';

export const SUMMARY_NODE_ID_TOP_LEVEL = 'summary_node_top_level';
export const RESULT_ARTIFACTS_ROOT_ID = 'result_artifacts_root';
export const INVOCATION_ARTIFACTS_ROOT_ID = 'invocation_artifacts_root';

let nodeIdCounterForTree = 0;

export function addArtifactsToNodeChildren(
  artifacts: readonly Artifact[],
  parentNodeChildren: CustomArtifactTreeNode[],
  baseLevel: number,
  pathPrefixSeed: string,
): void {
  const artifactMap = new Map<string, CustomArtifactTreeNode>();
  nodeIdCounterForTree = 0;

  artifacts.forEach((art) => {
    const pathParts = art.artifactId.split('/');
    let currentLevelChildrenList = parentNodeChildren;
    let currentPathForMapKey = pathPrefixSeed;
    let currentLevelForNode = baseLevel;

    pathParts.forEach((part, index) => {
      currentPathForMapKey += `/${part}`;
      const isLeafPart = index === pathParts.length - 1;

      let node = artifactMap.get(currentPathForMapKey);
      if (!node) {
        node = {
          id: isLeafPart
            ? art.name
            : `folder_${currentPathForMapKey}_${nodeIdCounterForTree++}`,
          name: part,
          level: currentLevelForNode,
          isLeaf: isLeafPart,
          artifactPb: isLeafPart ? art : undefined,
          children: isLeafPart ? undefined : [],
        };
        artifactMap.set(currentPathForMapKey, node);

        const existingNode = currentLevelChildrenList.find(
          (n) =>
            n.name === part && n.level === currentLevelForNode && !n.isLeaf,
        );
        if (existingNode && !isLeafPart) {
          node = existingNode;
        } else {
          currentLevelChildrenList.push(node);
        }
      }
      if (node.children) {
        currentLevelChildrenList = node.children;
      }
      currentLevelForNode++;
    });
  });
}

export function buildCustomArtifactTreeNodes(
  currentResult?: TestResult,
  testResultArtifacts?: readonly Artifact[],
  invocationScopeArtifacts?: readonly Artifact[],
): CustomArtifactTreeNode[] {
  const rootDisplayNodes: CustomArtifactTreeNode[] = [];
  nodeIdCounterForTree = 0;

  if (currentResult?.summaryHtml) {
    rootDisplayNodes.push({
      id: SUMMARY_NODE_ID_TOP_LEVEL,
      name: 'Summary',
      isSummary: true,
      isLeaf: true,
      level: 0,
    });
  }

  if (testResultArtifacts && testResultArtifacts.length > 0) {
    const resultArtifactsRoot: CustomArtifactTreeNode = {
      id: RESULT_ARTIFACTS_ROOT_ID,
      name: 'Test Result Artifacts',
      level: 0,
      children: [],
      isRootChild: true,
    };
    addArtifactsToNodeChildren(
      testResultArtifacts,
      resultArtifactsRoot.children!,
      1,
      RESULT_ARTIFACTS_ROOT_ID,
    );
    if (resultArtifactsRoot.children!.length > 0) {
      rootDisplayNodes.push(resultArtifactsRoot);
    }
  }

  if (invocationScopeArtifacts && invocationScopeArtifacts.length > 0) {
    const invocationArtifactsRoot: CustomArtifactTreeNode = {
      id: INVOCATION_ARTIFACTS_ROOT_ID,
      name: 'Invocation Artifacts',
      level: 0,
      children: [],
      isRootChild: true,
    };
    addArtifactsToNodeChildren(
      invocationScopeArtifacts,
      invocationArtifactsRoot.children!,
      1,
      INVOCATION_ARTIFACTS_ROOT_ID,
    );
    if (invocationArtifactsRoot.children!.length > 0) {
      rootDisplayNodes.push(invocationArtifactsRoot);
    }
  }
  return rootDisplayNodes;
}
