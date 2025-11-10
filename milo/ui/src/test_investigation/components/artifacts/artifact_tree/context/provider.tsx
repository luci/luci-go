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
// WITHOUT WARRANTIES OR CONDITIONS OF ANY, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { ReactNode, useCallback, useEffect, useMemo, useState } from 'react';

import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';

import { ArtifactTreeNodeData, SelectedArtifactSource } from '../../types';

import { ArtifactFiltersContext } from './context';

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

interface ArtifactFilterProviderProps {
  children: ReactNode;
  resultArtifacts: readonly Artifact[];
  invArtifacts: readonly Artifact[];
}

export function ArtifactFilterProvider({
  children,
  resultArtifacts,
  invArtifacts,
}: ArtifactFilterProviderProps) {
  const [searchTerm, setSearchTerm] = useState('');
  const [debouncedSearchTerm, setDebouncedSearchTerm] = useState('');

  const [artifactTypes, setArtifactTypes] = useState<string[]>([]);
  const [crashTypes, setCrashTypes] = useState<string[]>([]);
  const [showCriticalCrashes, setShowCriticalCrashes] = useState(false);
  const [hideAutomationFiles, setHideAutomationFiles] = useState(false);
  const [hideEmptyFolders, setHideEmptyFolders] = useState(false);
  const [showOnlyFoldersWithError, setShowOnlyFoldersWithError] =
    useState(false);

  const [isFilterPanelOpen, setIsFilterPanelOpen] = useState(false);

  const handleClearFilters = () => {
    setArtifactTypes([]);
    setCrashTypes([]);
    setShowCriticalCrashes(false);
    setHideAutomationFiles(false);
    setHideEmptyFolders(false);
    setShowOnlyFoldersWithError(false);
  };

  useEffect(() => {
    const timerId = setTimeout(() => {
      setDebouncedSearchTerm(searchTerm);
    }, 300);

    return () => {
      clearTimeout(timerId);
    };
  }, [searchTerm]);

  const availableArtifactTypes = useMemo(() => {
    const allArtifacts = [...resultArtifacts, ...invArtifacts];
    const types = new Set<string>();
    for (const artifact of allArtifacts) {
      if (artifact.artifactType) {
        types.add(artifact.artifactType);
      }
    }
    return Array.from(types).sort();
  }, [resultArtifacts, invArtifacts]);

  const filterArtifactList = useCallback(
    (artifacts: readonly Artifact[]) => {
      let filtered = artifacts;

      if (debouncedSearchTerm) {
        filtered = filtered.filter((artifact) =>
          artifact.artifactId
            .toLowerCase()
            .includes(debouncedSearchTerm.toLocaleLowerCase()),
        );
      }

      if (artifactTypes.length > 0) {
        const selectedTypes = new Set(artifactTypes);
        filtered = filtered.filter((artifact) =>
          selectedTypes.has(artifact.artifactType),
        );
      }

      return filtered;
    },
    [debouncedSearchTerm, artifactTypes],
  );

  const filteredResultArtifacts = useMemo(
    () => filterArtifactList(resultArtifacts),
    [resultArtifacts, filterArtifactList],
  );
  const filteredInvArtifacts = useMemo(
    () => filterArtifactList(invArtifacts),
    [invArtifacts, filterArtifactList],
  );

  const initialArtifactsTree = useMemo(() => {
    return buildArtifactsTree(filteredResultArtifacts, filteredInvArtifacts);
  }, [filteredResultArtifacts, filteredInvArtifacts]);

  const finalArtifactsTree = useMemo(() => {
    if (!hideEmptyFolders) {
      return initialArtifactsTree;
    }

    function pruneEmptyFolders(
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

    return pruneEmptyFolders(initialArtifactsTree);
  }, [initialArtifactsTree, hideEmptyFolders]);

  const contextValue = useMemo(
    () => ({
      searchTerm,
      setSearchTerm,
      debouncedSearchTerm,
      artifactTypes,
      setArtifactTypes,
      crashTypes,
      setCrashTypes,
      showCriticalCrashes,
      setShowCriticalCrashes,
      hideAutomationFiles,
      setHideAutomationFiles,
      hideEmptyFolders,
      setHideEmptyFolders,
      showOnlyFoldersWithError,
      setShowOnlyFoldersWithError,
      onClearFilters: handleClearFilters,
      availableArtifactTypes,
      finalArtifactsTree,
      isFilterPanelOpen,
      setIsFilterPanelOpen,
    }),
    [
      searchTerm,
      debouncedSearchTerm,
      artifactTypes,
      crashTypes,
      showCriticalCrashes,
      hideAutomationFiles,
      hideEmptyFolders,
      showOnlyFoldersWithError,
      availableArtifactTypes,
      finalArtifactsTree,
      isFilterPanelOpen,
    ],
  );

  return (
    <ArtifactFiltersContext.Provider value={contextValue}>
      {children}
    </ArtifactFiltersContext.Provider>
  );
}
