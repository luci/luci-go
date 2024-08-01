// Copyright 2023 The LUCI Authors.
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
import { ReactNode, useEffect, useRef, useState } from 'react';

import { OutputCommit } from '@/gitiles/types';

import { CommitContent } from './commit_content';
import { StyledTableRow } from './common';
import {
  useDefaultExpanded,
  useExpandStateStore,
  useTableRowIndex,
} from './context';
import {
  CommitProvider,
  ExpandedProvider,
  SetExpandedProvider,
} from './providers';

export interface CommitTableRowProps {
  /**
   * The commit to be rendered in this row.
   *
   * Use `null` to signal that the commit is still being loaded.
   */
  readonly commit: OutputCommit | null;
  /**
   * The content to be rendered when expanded.
   *
   *  Defaults to `<CommitContent />`.
   */
  readonly content?: ReactNode;
  readonly children: ReactNode;
}

export function CommitTableRow({
  commit,
  content,
  children,
}: CommitTableRowProps) {
  const tableRowIndex = useTableRowIndex();
  const expandStateStore = useExpandStateStore();

  const defaultExpanded = useDefaultExpanded();
  const [expanded, setExpanded] = useState(
    // Recover expand state from the external store if it's stored there.
    // This allow the commit row to keep its state when it's unmounted
    // (e.g. due to virtual windowing).
    () =>
      (tableRowIndex === undefined
        ? undefined
        : expandStateStore?.[tableRowIndex]) ?? defaultExpanded,
  );

  // Sync the expand state to the default expand state whenever the default
  // expand state changes.
  const isFirstCall = useRef(true);
  useEffect(() => {
    // Skip the assignment in the first rendering call so we don't overwrite the
    // state recovered from the external store immediately.
    if (isFirstCall.current) {
      isFirstCall.current = false;
      return;
    }
    setExpanded(defaultExpanded);
  }, [defaultExpanded]);

  // Store the expand state in the external store so the state can be recovered
  // when the component is re-mounted.
  useEffect(() => {
    if (tableRowIndex === undefined || expandStateStore === undefined) {
      return;
    }
    expandStateStore[tableRowIndex] = expanded;
  }, [expanded, tableRowIndex, expandStateStore]);

  // We do not need `wasExpanded` to be a state because it could only change
  // when another state, `expanded`, is updated. Keeping it in a ref reduces
  // 1 rerendering cycle.
  const wasExpanded = useRef(expanded);
  wasExpanded.current ||= expanded;

  return (
    <SetExpandedProvider value={setExpanded}>
      <ExpandedProvider value={expanded}>
        {/* Pass commit to cells via context so composing a row require less
         ** boilerplate. */}
        <CommitProvider value={commit}>
          <StyledTableRow>{children}</StyledTableRow>
          {/* Always render the content row to DOM to ensure a stable DOM
           ** structure.
           **/}
          <TableRow>
            <TableCell
              data-testid="content-cell"
              colSpan={100}
              sx={{ display: expanded ? '' : 'none', 'tr > &': { padding: 0 } }}
            >
              {/* Don't always render the content because the content
               ** (especially a customized content) could be expensive to
               ** render.
               **
               ** Once the content was rendered, keep it in the tree so we don't
               ** constantly trigger mounting/unmounting events.
               **/}
              {wasExpanded.current ? content || <CommitContent /> : <></>}
            </TableCell>
          </TableRow>
        </CommitProvider>
      </ExpandedProvider>
    </SetExpandedProvider>
  );
}
