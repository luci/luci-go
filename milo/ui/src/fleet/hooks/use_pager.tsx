// Copyright 2026 The LUCI Authors.
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

import { useCallback, useMemo } from 'react';

import {
  getCurrentPageIndex,
  getPageSize,
  getPageToken,
  nextPageTokenUpdater,
  pageSizeUpdater,
  PagerContext,
  prevPageTokenUpdater,
} from '@/common/components/params_pager';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

/**
 * Hook for managing pagination state and actions synced with URL search parameters.
 * Returns both the current pagination values (pageSize, pageToken, pageIndex) and
 * the navigation handlers (goToNextPage, goToPrevPage, onRowsPerPageChange).
 */
export function usePager(pagerCtx: PagerContext) {
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const state = useMemo(
    () => ({
      pageSize: getPageSize(pagerCtx, searchParams),
      pageToken: getPageToken(pagerCtx, searchParams),
      pageIndex: getCurrentPageIndex(pagerCtx),
    }),
    [pagerCtx, searchParams],
  );

  const goToNextPage = useCallback(
    (token: string) =>
      setSearchParams((prev) => nextPageTokenUpdater(pagerCtx, token)(prev)),
    [pagerCtx, setSearchParams],
  );

  const goToPrevPage = useCallback(
    () => setSearchParams((prev) => prevPageTokenUpdater(pagerCtx)(prev)),
    [pagerCtx, setSearchParams],
  );

  const onRowsPerPageChange = useCallback(
    (size: number) => setSearchParams(pageSizeUpdater(pagerCtx, size)),
    [pagerCtx, setSearchParams],
  );

  return {
    ...state,
    goToNextPage,
    goToPrevPage,
    onRowsPerPageChange,
  };
}
