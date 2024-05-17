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

import { TableCell, TableRow } from '@mui/material';
import { useEffect, useRef, useState } from 'react';
import { useDebounce, useIntersection } from 'react-use';

import { DotSpinner } from '@/generic_libs/components/dot_spinner';

import { SegmentContentCell } from './segment_column';

export interface LoadingRowProps {
  readonly loadedPageCount: number;
  readonly nextCommitPosition: string | null;
  /**
   * Callback to notify the parent to load the next page.
   *
   * The next page should be loaded when the <LoadingRow /> is visible in the
   * viewport for the first time after the previous page was loaded.
   */
  readonly loadNextPage: () => void;
}

/**
 * A row with a loading spinner. Load the next page when it's visible in the
 * viewport. This helps implementing infinite scrolling.
 */
export function LoadingRow({
  loadedPageCount,
  nextCommitPosition,
  loadNextPage,
}: LoadingRowProps) {
  const pageTailRef = useRef<HTMLTableRowElement>(null);
  const loadNextPageRef = useRef(loadNextPage);
  loadNextPageRef.current = loadNextPage;

  const intersection = useIntersection(pageTailRef, {});
  const inView = Boolean(intersection?.isIntersecting);

  const [nextPageIndex, setNextPageIndex] = useState(0);
  // When the new page is just loaded, it takes some time to render the new
  // content. Therefore the loading row is still in the viewport.
  // We need to add a small delay before loading the next page
  // otherwise we will always load 2 pages back-to-back.
  useDebounce(() => setNextPageIndex(loadedPageCount), 500, [loadedPageCount]);

  // Load the next page when the loading row enters the view and the next page
  // hasn't been requested.
  const requestedPageIndexRef = useRef(0);
  useEffect(() => {
    if (!inView) {
      return;
    }
    if (requestedPageIndexRef.current === nextPageIndex) {
      return;
    }
    loadNextPageRef.current();
    requestedPageIndexRef.current = nextPageIndex;
  }, [nextPageIndex, inView]);

  return (
    <TableRow ref={pageTailRef}>
      {
        // Render the <SegmentContentCell /> for the next commit position so
        // the loading spinner does not overlap the segment column.
        nextCommitPosition ? (
          <SegmentContentCell position={nextCommitPosition} />
        ) : (
          <></>
        )
      }
      <TableCell colSpan={100} height="35px">
        Loading <DotSpinner />
      </TableCell>
    </TableRow>
  );
}
