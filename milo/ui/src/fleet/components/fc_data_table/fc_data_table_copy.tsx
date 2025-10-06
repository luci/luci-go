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

export function FCDataTableCopy<T extends MRT_RowData>({
  table,
}: {
  table: MRT_TableInstance<T>;
}) {
  const [showCopySuccess, setShowCopySuccess] = useState(false);

  const rows = table.getSelectedRowModel().rows;
  if (rows.length === 0) return <div />;

  return (
    <>
      <Button
        onClick={() => {
          const keys = Object.keys(rows[0]?.original);
          const textToCopy =
            keys.join('\t') +
            '\n' +
            rows
              .map((r) => keys.map((key) => r.original[key]).join('\t'))
              .join('\n');

          setShowCopySuccess(true);
          navigator.clipboard.writeText(textToCopy);
        }}
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
