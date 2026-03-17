// Copyright 2026 The LUCI Authors.
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

import { Theme } from '@mui/material/styles';
import { MRT_TableOptions } from 'material-react-table';

/**
 * Common configuration for Material React Table.
 */
export const COMMON_MRT_CONFIG: Partial<
  MRT_TableOptions<Record<string, number | string | null | undefined>>
> = {
  enableTopToolbar: false,
  enableColumnActions: false,
  enableGlobalFilter: false,
  enableDensityToggle: false,
  enableFullScreenToggle: false,
  enableHiding: false,
  enableFilters: false,
  layoutMode: 'semantic',
  muiTableHeadRowProps: {
    sx: {
      backgroundColor: (theme: Theme) => theme.palette.divider,
      borderBottom: '1px solid',
      borderColor: 'divider',
      boxShadow: 'none',
    },
  },
  muiTableHeadCellProps: {
    sx: {
      fontWeight: (theme: Theme) => theme.typography.fontWeightBold,
      fontSize: (theme: Theme) => theme.typography.caption.fontSize,
    },
  },
  muiTableBodyProps: {
    sx: {
      '& tr:nth-of-type(odd) > td': {
        backgroundColor: 'transparent',
      },
      '& td': {
        borderBottom: '1px solid',
        borderColor: 'divider',
        py: 1,
      },
    },
  },
  muiTableContainerProps: {
    sx: { overflowX: 'auto' },
  },
  muiTablePaperProps: {
    sx: {
      m: 0,
      borderRadius: 0,
      boxShadow: 'none',
      border: 'none',
    },
    elevation: 0,
  },
};
