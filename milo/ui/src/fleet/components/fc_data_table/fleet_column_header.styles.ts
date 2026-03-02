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

import { SxProps, Theme } from '@mui/material';

import { colors } from '@/fleet/theme/colors';

export type Density = 'spacious' | 'comfortable' | 'compact';

export const getDensityPadding = (density: Density) => {
  const vertical = density === 'spacious' ? 16 : density === 'compact' ? 8 : 12;
  const horizontal =
    density === 'spacious' ? 12 : density === 'compact' ? 4 : 8;
  return { vertical, horizontal };
};

export const fleetTableHeaderSx: SxProps<Theme> = {
  '--header-bg-color': colors.grey[100],
  // Use && to increase specificity and avoid !important where possible
  '&&': {
    backgroundColor: 'var(--header-bg-color)',
    fontWeight: 500,
    '&.column-highlight': {
      backgroundColor: `${colors.blue[50]} !important`,
    },
  },
  position: 'relative',
  overflow: 'visible',
  verticalAlign: 'middle',
  '&&.column-highlight .fleet-column-actions': {
    backgroundColor: `${colors.blue[50]}`,
  },
  '& .Mui-TableHeadCell-Content': {
    width: '100%',
    height: '100%',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    position: 'static',
  },
  // Visibility logic for icons
  '&& .fleet-column-actions.is-sorted': {
    display: 'flex',
    pointerEvents: 'auto',
    flexShrink: 0,
  },
  '&&:hover .fleet-column-actions': {
    display: 'flex',
    pointerEvents: 'auto',
    flexShrink: 0,
  },
  '&&:focus-within .fleet-column-actions': {
    display: 'flex',
    pointerEvents: 'auto',
    flexShrink: 0,
  },
  // Base suppression for all icons until shown by the container above
  '&& .MuiTableSortLabel-root, && .ColumnActionsMenuButton': {
    display: 'none',
  },
  // Target the SORT arrows specifically for circular hover
  '&& .fleet-column-actions.is-sorted .MuiTableSortLabel-root, &&:hover .fleet-column-actions .MuiTableSortLabel-root':
    {
      display: 'flex',
      borderRadius: '50%',
      padding: '4px',
      width: '24px',
      height: '24px',
      boxSizing: 'border-box',
      alignItems: 'center',
      justifyContent: 'center',
      transition: 'background-color 0.2s',
      '&:hover': {
        backgroundColor: 'rgba(0, 0, 0, 0.05)',
      },
      '& .MuiTableSortLabel-icon': {
        margin: 0,
        fontSize: '18px',
      },
    },
  // Reduce the size of the 3-dot menu icon to match the others
  '&& .ColumnActionsMenuButton': {
    width: '24px !important',
    height: '24px !important',
    padding: '4px !important',
    minWidth: 'unset !important',
    boxSizing: 'border-box !important',
    borderRadius: '50% !important',
    transition: 'background-color 0.2s',
    '&:hover': {
      backgroundColor: 'rgba(0, 0, 0, 0.05)',
    },
    '& .MuiSvgIcon-root': {
      fontSize: '18px !important',
    },
  },
  '&&:hover .fleet-column-actions .ColumnActionsMenuButton, && .fleet-column-actions:focus-within .ColumnActionsMenuButton':
    {
      display: 'inline-flex',
    },
  // Target our custom INFO icon for circular hover
  '& .fleet-info-icon': {
    borderRadius: '50%',
    padding: '4px',
    width: '24px',
    height: '24px',
    boxSizing: 'border-box',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    transition: 'background-color 0.2s',
    '&:hover': {
      backgroundColor: 'rgba(0, 0, 0, 0.05)',
    },
    '& .MuiSvgIcon-root': {
      fontSize: '16px', // Matches InfoTooltip fontSize override
    },
  },
  '&& .Mui-TableHeadCell-Content-Actions': {
    flexShrink: 0,
  },
  '&& .Mui-TableHeadCell-Content-Wrapper': {
    overflow: 'visible',
    minWidth: 0,
    width: '100%',
  },
  '&& .Mui-TableHeadCell-Content-Labels': {
    overflow: 'visible',
    minWidth: 0,
    flexShrink: 1,
  },
  '&& .Mui-TableHeadCell-ResizeHandle-Wrapper': {
    zIndex: 2000,
    height: '100%',
    top: 0,
    right: '0px !important', // Keep the hit-target strictly inside the cell bounds
    width: '8px !important', // Make the hit-target wider
    marginRight: '0px !important',
    padding: '0 !important',
    visibility: 'visible !important',
    opacity: '1 !important',
    display: 'flex !important',
    alignItems: 'center !important',
    justifyContent: 'center !important',
  },
  '&& .Mui-TableHeadCell-ResizeHandle-Divider': {
    backgroundColor: `${colors.grey[400]} !important`,
    borderWidth: '0 !important',
    border: 'none !important',
    opacity: '1 !important',
    visibility: 'visible !important',
    height: '24px !important',
    position: 'static !important',
    alignSelf: 'center !important',
    width: '2px !important',
    borderRadius: '1px !important',
    transition: 'all 0.2s',
    '&:hover, &[data-isresizing="true"]': {
      backgroundColor: `${colors.blue[500]} !important`,
      width: '4px !important',
      borderRadius: '2px !important',
    },
  },
};
