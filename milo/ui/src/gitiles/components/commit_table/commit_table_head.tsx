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

import { SxProps, TableHead, TableRow, Theme } from '@mui/material';
import { CSSProperties, ForwardedRef, ReactNode, forwardRef } from 'react';

export interface CommitTableHeadProps {
  readonly sx?: SxProps<Theme>;
  readonly style?: CSSProperties;
  readonly children?: ReactNode;
}

export const CommitTableHead = forwardRef(function CommitTableHead(
  { sx, children, style, ...props }: CommitTableHeadProps,
  ref: ForwardedRef<HTMLTableSectionElement>,
) {
  return (
    <TableHead
      {...props}
      ref={ref}
      // Prevent `top` from MUI-generated class being overridden by style
      // assigned by `react-virtuoso`.
      style={{ ...style, top: undefined }}
      sx={{
        position: 'sticky',
        top: 'var(--accumulated-top)',
        backgroundColor: 'white',
        zIndex: 2,
        ...sx,
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
    </TableHead>
  );
});
