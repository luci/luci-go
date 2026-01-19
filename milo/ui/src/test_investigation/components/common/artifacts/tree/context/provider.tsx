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

import { ReactNode, useCallback, useEffect, useMemo, useState } from 'react';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { ArtifactFiltersContext } from './context';

interface ArtifactFilterProviderProps {
  children: ReactNode;
}

export function ArtifactFilterProvider({
  children,
}: ArtifactFilterProviderProps) {
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  // Search Param mapping:
  // q: searchTerm
  // types: artifactTypes (comma separated)
  // crash: crashTypes (comma separated)
  // critical: showCriticalCrashes (bool)
  // no_auto: hideAutomationFiles (bool)
  // no_empty: hideEmptyFolders (bool)
  // err_only: showOnlyFoldersWithError (bool)
  const searchTerm = searchParams.get('q') || '';
  const setSearchTerm = useCallback(
    (term: string) => {
      setSearchParams((params) => {
        if (term) {
          params.set('q', term);
        } else {
          params.delete('q');
        }
        return params;
      });
    },
    [setSearchParams],
  );

  const [debouncedSearchTerm, setDebouncedSearchTerm] = useState(searchTerm);

  const artifactTypes = useMemo(
    () => searchParams.get('types')?.split(',').filter(Boolean) || [],
    [searchParams],
  );
  const setArtifactTypes = useCallback(
    (types: string[]) => {
      setSearchParams((params) => {
        if (types.length > 0) {
          params.set('types', types.join(','));
        } else {
          params.delete('types');
        }
        return params;
      });
    },
    [setSearchParams],
  );

  const crashTypes = useMemo(
    () => searchParams.get('crash')?.split(',').filter(Boolean) || [],
    [searchParams],
  );
  const setCrashTypes = useCallback(
    (types: string[]) => {
      setSearchParams((params) => {
        if (types.length > 0) {
          params.set('crash', types.join(','));
        } else {
          params.delete('crash');
        }
        return params;
      });
    },
    [setSearchParams],
  );

  const showCriticalCrashes = searchParams.get('critical') === 'true';
  const setShowCriticalCrashes = useCallback(
    (show: boolean) => {
      setSearchParams((params) => {
        if (show) {
          params.set('critical', 'true');
        } else {
          params.delete('critical');
        }
        return params;
      });
    },
    [setSearchParams],
  );

  const hideAutomationFiles = searchParams.get('no_auto') === 'true';
  const setHideAutomationFiles = useCallback(
    (hide: boolean) => {
      setSearchParams((params) => {
        if (hide) {
          params.set('no_auto', 'true');
        } else {
          params.delete('no_auto');
        }
        return params;
      });
    },
    [setSearchParams],
  );

  const hideEmptyFolders = searchParams.get('no_empty') !== 'false';
  const setHideEmptyFolders = useCallback(
    (hide: boolean) => {
      setSearchParams((params) => {
        if (hide) {
          params.delete('no_empty');
        } else {
          params.set('no_empty', 'false');
        }
        return params;
      });
    },
    [setSearchParams],
  );

  const showOnlyFoldersWithError = searchParams.get('err_only') === 'true';
  const setShowOnlyFoldersWithError = useCallback(
    (show: boolean) => {
      setSearchParams((params) => {
        if (show) {
          params.set('err_only', 'true');
        } else {
          params.delete('err_only');
        }
        return params;
      });
    },
    [setSearchParams],
  );

  const [isFilterPanelOpen, setIsFilterPanelOpen] = useState(false);
  const [availableArtifactTypes, setAvailableArtifactTypes] = useState<
    readonly string[]
  >([]);

  const handleClearFilters = useCallback(() => {
    setSearchParams((params) => {
      params.delete('types');
      params.delete('crash');
      params.delete('critical');
      params.delete('no_auto');
      params.delete('no_empty');
      params.delete('err_only');
      return params;
    });
  }, [setSearchParams]);

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
      handleClearFilters,
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
