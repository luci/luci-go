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

import { TablePaginationProps } from '@mui/material';
import {
  GridSlots,
  GridPagination,
  useGridApiContext,
  PropsFromSlot,
  useGridSelector,
  gridFilteredTopLevelRowCountSelector,
} from '@mui/x-data-grid';
import * as React from 'react';

import {
  getCurrentPageIndex,
  getPrevFullRowCount,
  nextPageTokenUpdater,
  PagerContext,
  pageSizeUpdater,
  prevPageTokenUpdater,
} from '@/common/components/params_pager';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

declare module '@mui/x-data-grid' {
  interface ToolbarPropsOverrides {
    setColumnsButtonEl: (element: HTMLButtonElement | null) => void;
  }

  interface PaginationPropsOverrides {
    pagerCtx: PagerContext;
    nextPageToken: string;
    totalRowCount?: number;
  }
}

export function Pagination({
  pagerCtx,
  nextPageToken,
  totalRowCount,
}: PropsFromSlot<GridSlots['pagination']>) {
  const apiRef = useGridApiContext();
  const visibleTopLevelRowCount = useGridSelector(
    apiRef,
    gridFilteredTopLevelRowCountSelector,
  );
  const [_, setSearchParams] = useSyncedSearchParams();
  const hasNextPage = nextPageToken !== '';

  const labelDisplayedRows = () => {
    // When reloading a page beyond the first, displayed counts may be
    // unexpected. This is because previous page counts reset, showing
    // only the first page's count. This behavior is largely intended,
    // as navigating back also leads to the first page. Omitting counts
    // may be a solution if current behavior is confusing.
    const from = getPrevFullRowCount(pagerCtx) + 1;
    const to = from + visibleTopLevelRowCount - 1;
    if (totalRowCount) {
      return `${from}-${to} of ${totalRowCount}`;
    } else {
      return `${from}-${to} of ${hasNextPage ? `more than ${to}` : to}`;
    }
  };

  const handlePageSizeChange = React.useCallback(
    (event: React.ChangeEvent<HTMLTextAreaElement | HTMLInputElement>) => {
      const pageSize = Number(event.target.value);
      setSearchParams(pageSizeUpdater(pagerCtx, pageSize));
      apiRef.current.setPageSize(pageSize);
    },
    [apiRef, pagerCtx, setSearchParams],
  );

  const handlePageChange = React.useCallback<
    TablePaginationProps['onPageChange']
  >(
    (_, page) => {
      const currentPage = getCurrentPageIndex(pagerCtx);
      const isPrevPage = page < currentPage;
      const isNextPage = page > currentPage;

      if (isPrevPage) {
        setSearchParams(prevPageTokenUpdater(pagerCtx));
      } else if (isNextPage) {
        setSearchParams(nextPageTokenUpdater(pagerCtx, nextPageToken));
      }
      apiRef.current.setPage(page);
    },
    [apiRef, pagerCtx, nextPageToken, setSearchParams],
  );

  return (
    <GridPagination
      labelDisplayedRows={labelDisplayedRows}
      onRowsPerPageChange={handlePageSizeChange}
      onPageChange={handlePageChange}
      slotProps={{
        actions: {
          nextButton: {
            disabled: !hasNextPage,
          },
        },
        select: {
          MenuProps: {
            sx: { zIndex: 1401 }, // luci's cookie_consent_bar is 14000
          },
        },
      }}
    />
  );
}
