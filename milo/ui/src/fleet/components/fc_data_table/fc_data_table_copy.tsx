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

import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { Button } from '@mui/material';
import { MRT_RowData, MRT_TableInstance } from 'material-react-table';
import { useState } from 'react';

import { CopySnackbar } from '../actions/copy/copy_snackbar';
import { useShortcut } from '../shortcut_provider';

const sanitizeValue = (value: unknown): string => {
  if (value === null || value === undefined) return '';
  const str = typeof value === 'object' ? JSON.stringify(value) : String(value);
  // Replace tabs and newlines with spaces to preserve TSV integrity
  return str.replace(/[\t\n\r]/g, ' ');
};

export function FCDataTableCopy<T extends MRT_RowData>({
  table,
}: {
  table: MRT_TableInstance<T>;
}) {
  const [showCopySuccess, setShowCopySuccess] = useState(false);

  const rows = table.getSelectedRowModel().rows;

  const copy = () => {
    const currentRows = table.getSelectedRowModel().rows;
    if (currentRows.length === 0) return;

    const visibleColumns = table.getVisibleLeafColumns();
    const columnsToCopy = visibleColumns.filter((col) => {
      const def = col.columnDef as {
        accessorKey?: string;
        accessorFn?: unknown;
      };
      return def.accessorKey || def.accessorFn;
    });

    const headers = columnsToCopy.map((col) => {
      const header = col.columnDef.header;
      return typeof header === 'string' ? header : col.id;
    });

    const textToCopy =
      headers.join('\t') +
      '\n' +
      currentRows
        .map((r) =>
          columnsToCopy
            .map((col) => sanitizeValue(r.getValue(col.id)))
            .join('\t'),
        )
        .join('\n');

    setShowCopySuccess(true);
    navigator.clipboard.writeText(textToCopy);
  };

  useShortcut('Copy the selected rows', ['Ctrl+c', 'Cmd+c'], copy, {
    enabled: rows.length > 0,
  });

  if (rows.length === 0) return <div />;
  return (
    <>
      <Button
        onClick={copy}
        startIcon={<ContentCopyIcon />}
        aria-label="Copy selected rows"
      >
        Copy
      </Button>
      <CopySnackbar
        open={showCopySuccess}
        onClose={() => setShowCopySuccess(false)}
      />
    </>
  );
}
