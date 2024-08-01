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

import { SxProps, Theme } from '@mui/material';
import { ForwardedRef, forwardRef, useEffect, useRef, useState } from 'react';
import {
  TableVirtuoso,
  TableVirtuosoHandle,
  TableVirtuosoProps,
} from 'react-virtuoso';

import { CommitTableHead } from '@/gitiles/components/commit_table';
import { CommitTableBody } from '@/gitiles/components/commit_table/commit_table_body';

import { StyledTable } from './common';
import {
  DefaultExpandedProvider,
  ExpandStateStoreProvider,
  RepoUrlProvider,
  SetDefaultExpandedProvider,
  TableRowPropsProvider,
  TableSxProvider,
} from './providers';

function getItemSize(el: HTMLElement, field: 'offsetWidth' | 'offsetHeight') {
  switch (field) {
    case 'offsetWidth':
      return el.getBoundingClientRect().width;
    case 'offsetHeight':
      return (
        el.getBoundingClientRect().height +
        // We render two rows for each item. So we need to add the next row's
        // height when calculating the height of the item.
        el.nextElementSibling!.getBoundingClientRect().height
      );
  }
}

export interface VirtualizedCommitTableProps
  extends Omit<
    TableVirtuosoProps<undefined, undefined>,
    'itemSize' | 'components'
  > {
  readonly repoUrl: string;
  readonly initDefaultExpanded?: boolean;
  readonly onDefaultExpandedChanged?: (expand: boolean) => void;
  readonly sx?: SxProps<Theme>;
}

export const VirtualizedCommitTable = forwardRef(
  function VirtualizedCommitTable(
    {
      repoUrl,
      initDefaultExpanded = false,
      onDefaultExpandedChanged = () => {
        // Noop by default.
      },
      sx,
      ...props
    }: VirtualizedCommitTableProps,
    ref: ForwardedRef<TableVirtuosoHandle>,
  ) {
    const [defaultExpanded, setDefaultExpanded] = useState(initDefaultExpanded);
    const [expandStateStore] = useState<boolean[]>([]);

    const startIndexRef = useRef(0);

    const onDefaultExpandedChangedRef = useRef(onDefaultExpandedChanged);
    onDefaultExpandedChangedRef.current = onDefaultExpandedChanged;
    const isFirstCall = useRef(true);
    useEffect(() => {
      // Skip the first call because the default state were not changed.
      if (isFirstCall.current) {
        isFirstCall.current = false;
        return;
      }
      onDefaultExpandedChangedRef.current(defaultExpanded);

      // Do not reset the expansion state for items captured by the top padding.
      // Otherwise this will lead to flickers because the size of the item no
      // longer matches the what react-virtuoso recorded.
      expandStateStore.length = startIndexRef.current + 1;
    }, [defaultExpanded, expandStateStore]);

    return (
      <ExpandStateStoreProvider value={expandStateStore}>
        <SetDefaultExpandedProvider value={setDefaultExpanded}>
          <DefaultExpandedProvider value={defaultExpanded}>
            <RepoUrlProvider value={repoUrl}>
              <TableSxProvider value={sx}>
                <TableVirtuoso
                  {...props}
                  ref={ref}
                  itemSize={getItemSize}
                  components={{
                    Table: StyledTable,
                    TableHead: CommitTableHead,
                    TableBody: CommitTableBody,
                    TableRow: TableRowPropsProvider,
                  }}
                  rangeChanged={(r) => {
                    startIndexRef.current = r.startIndex;
                    props.rangeChanged?.(r);
                  }}
                />
              </TableSxProvider>
            </RepoUrlProvider>
          </DefaultExpandedProvider>
        </SetDefaultExpandedProvider>
      </ExpandStateStoreProvider>
    );
  },
);
