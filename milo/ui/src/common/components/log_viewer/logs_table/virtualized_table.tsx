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

import { Table, TableRow } from '@mui/material';
import { blue, grey } from '@mui/material/colors';
import { ReactNode, forwardRef } from 'react';
import {
  ItemProps,
  TableComponents,
  TableVirtuoso,
  TableVirtuosoHandle,
} from 'react-virtuoso';

import { LogsTableEntry } from '../types';

const LogsTableBody = forwardRef<HTMLTableSectionElement>((props, ref) => (
  <tbody {...props} ref={ref} />
));
LogsTableBody.displayName = 'LogsTableBody';

interface Props {
  entries: LogsTableEntry[];
  onRowClick?: (
    index: number,
    event: React.MouseEvent<HTMLTableRowElement, MouseEvent>,
  ) => void;
  onRowMouseDown?: (
    index: number,
    event: React.MouseEvent<HTMLTableRowElement, MouseEvent>,
  ) => void;
  onRangeChanged?: () => void;
  getRowColor?: (entryId: string, index: number) => string;
  fixedHeaderContent?: () => ReactNode;
  fixedFooterContent?: () => ReactNode;
  rowContent: (index: number, row: LogsTableEntry) => ReactNode;
  initialTopMostItemIndex: number;
  disableVirtualization?: boolean;
}

export const VirtualizedTable = forwardRef<TableVirtuosoHandle | null, Props>(
  function VirtualizedTable(
    {
      entries,
      onRowClick,
      rowContent,
      initialTopMostItemIndex,
      disableVirtualization,
      fixedHeaderContent,
      fixedFooterContent,
      getRowColor,
      onRowMouseDown,
      onRangeChanged,
    }: Props,
    ref,
  ) {
    const VirtuosoTableComponents: TableComponents<LogsTableEntry> = {
      Table: (props) => (
        <Table
          stickyHeader
          {...props}
          sx={{
            borderCollapse: 'separate',
            tableLayout: 'fixed',
            overflow: 'auto',
          }}
        />
      ),
      TableRow: ({
        item,
        'data-index': index,
        ...props
      }: ItemProps<LogsTableEntry>) => (
        <TableRow
          hover
          data-testid={`logs-table-row-${index}`}
          onMouseDown={(event) =>
            onRowMouseDown && onRowMouseDown(index, event)
          }
          onClick={(event) => onRowClick && onRowClick(index, event)}
          sx={{
            backgroundColor:
              getRowColor?.(item.entryId, index) ??
              (index % 2 ? grey[200] : ''),
            '&:hover': {
              backgroundColor: `${blue[50]} !important`,
            },
          }}
          data-index={index}
          {...props}
        />
      ),
      TableBody: LogsTableBody,
    };

    return (
      <TableVirtuoso
        data-testid="logs-table"
        ref={ref}
        data={entries}
        totalCount={entries.length}
        components={VirtuosoTableComponents}
        fixedHeaderContent={fixedHeaderContent}
        fixedFooterContent={fixedFooterContent}
        itemContent={rowContent}
        rangeChanged={onRangeChanged}
        initialTopMostItemIndex={initialTopMostItemIndex}
        // Setting this to total count disables virtualization
        initialItemCount={disableVirtualization ? entries.length : undefined}
        key={disableVirtualization ? entries.length : undefined}
      />
    );
  },
);
