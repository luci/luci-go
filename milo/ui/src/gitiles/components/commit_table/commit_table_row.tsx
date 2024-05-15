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
import {
  CommitProvider,
  ExpandedProvider,
  SetExpandedProvider,
  useDefaultExpanded,
} from './context';

export interface CommitTableRowProps {
  readonly commit: OutputCommit;
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
  const defaultExpanded = useDefaultExpanded();
  const [expanded, setExpanded] = useState(() => defaultExpanded);
  useEffect(() => setExpanded(defaultExpanded), [defaultExpanded]);

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
          <TableRow
            sx={{
              '& > td': { whiteSpace: 'nowrap' },
              // Add a fixed height to allow children to use `height: 100%`.
              // The actual height of the row will expand to contain the
              // children.
              height: '1px',
            }}
          >
            {children}
          </TableRow>
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
