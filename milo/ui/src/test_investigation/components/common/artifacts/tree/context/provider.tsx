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

import { ReactNode, useEffect, useMemo, useState } from 'react';

import { ArtifactFiltersContext } from './context';

interface ArtifactFilterProviderProps {
  children: ReactNode;
}

export function ArtifactFilterProvider({
  children,
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
  const [availableArtifactTypes, setAvailableArtifactTypes] = useState<
    readonly string[]
  >([]);

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
      setAvailableArtifactTypes,
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
      isFilterPanelOpen,
    ],
  );

  return (
    <ArtifactFiltersContext.Provider value={contextValue}>
      {children}
    </ArtifactFiltersContext.Provider>
  );
}
