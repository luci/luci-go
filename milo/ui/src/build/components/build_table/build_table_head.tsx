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

import { LinearProgress, TableCell, TableHead, TableRow } from '@mui/material';
import { ReactNode } from 'react';

export interface BuildTableHeadProps {
  readonly showLoadingBar?: boolean;
  readonly children: ReactNode;
}

export function BuildTableHead({
  showLoadingBar = false,
  children,
}: BuildTableHeadProps) {
  return (
    <TableHead
      sx={{
        position: 'sticky',
        top: 'var(--accumulated-top)',
        backgroundColor: 'white',
        zIndex: 2,
        '& th': {
          border: 'none',
        },
      }}
    >
      <TableRow
        sx={{
          '& > th': {
            whiteSpace: 'nowrap',
          },
        }}
      >
        {children}
      </TableRow>
      <TableRow>
        <TableCell
          colSpan={100}
          className="divider"
          sx={{ '&.divider': { padding: '0' } }}
        >
          <LinearProgress
            value={100}
            variant={showLoadingBar ? 'indeterminate' : 'determinate'}
            color={showLoadingBar ? 'primary' : 'dividerLine'}
            sx={{ height: '1px' }}
          />
        </TableCell>
      </TableRow>
    </TableHead>
  );
}
