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

import { createContext, useContext } from 'react';

import { ArtifactTreeNodeData } from '../../types';

export interface ArtifactFiltersContextValue {
  // Search
  searchTerm: string;
  setSearchTerm: (value: string) => void;
  debouncedSearchTerm: string;

  // Filter States
  artifactTypes: string[];
  setArtifactTypes: (value: string[]) => void;
  crashTypes: string[];
  setCrashTypes: (value: string[]) => void;
  showCriticalCrashes: boolean;
  setShowCriticalCrashes: (value: boolean) => void;
  hideAutomationFiles: boolean;
  setHideAutomationFiles: (value: boolean) => void;
  hideEmptyFolders: boolean;
  setHideEmptyFolders: (value: boolean) => void;
  showOnlyFoldersWithError: boolean;
  setShowOnlyFoldersWithError: (value: boolean) => void;

  // Actions
  onClearFilters: () => void;

  // Derived Data
  availableArtifactTypes: readonly string[];
  finalArtifactsTree: readonly ArtifactTreeNodeData[];

  // UI State
  isFilterPanelOpen: boolean;
  setIsFilterPanelOpen: (value: boolean | ((prev: boolean) => boolean)) => void;
}

export const ArtifactFiltersContext =
  createContext<ArtifactFiltersContextValue | null>(null);

export function useArtifactFilters() {
  const context = useContext(ArtifactFiltersContext);
  if (context === null) {
    throw new Error(
      'useArtifactFilters must be used within an ArtifactFilterProvider',
    );
  }
  return context;
}
