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
import {
  borders,
  fleetColors,
  fontFamilies,
  fontSizes,
  iconSizes,
  radii,
} from '@/fleet/theme/tokens';

export type Density = 'spacious' | 'comfortable' | 'compact';

export const TABLE_HEADER_MIN_HEIGHT = '48px';
export const SELECT_COL_PADDING = '8px';

export const getDensityPadding = (density: Density) => {
  const vertical = density === 'spacious' ? 16 : density === 'compact' ? 4 : 12;
  const horizontal =
    density === 'spacious' ? 12 : density === 'compact' ? 4 : 8;
  return { vertical, horizontal };
};

export const fleetTableHeaderSx: SxProps<Theme> = {
  '--header-bg-color': fleetColors.surfaceTonal,
  // Use && to increase specificity and avoid !important where possible
  '&&': {
    backgroundColor: 'var(--header-bg-color)',
    borderBottom: borders.emphasis,
    fontFamily: fontFamilies.text,
    fontWeight: 600,
    fontSize: fontSizes.label,
    color: fleetColors.textSecondary,
    paddingTop: 'var(--cell-padding-vertical)',
    paddingBottom: 'var(--cell-padding-vertical)',
    paddingLeft: 'var(--cell-padding-horizontal)',
    paddingRight: 'var(--cell-padding-horizontal)',
    '&.select-col': {
      paddingLeft: SELECT_COL_PADDING,
      paddingRight: SELECT_COL_PADDING,
    },
    '&.column-highlight': {
      backgroundColor: fleetColors.primaryContainer,
      color: fleetColors.primary,
    },
  },
  transition: 'padding 0.2s',
  position: 'relative',
  overflow: 'visible',
  verticalAlign: 'middle',
  '&&.column-highlight .fleet-column-actions': {
    backgroundColor: fleetColors.primaryContainer,
  },
  '& .Mui-TableHeadCell-Content': {
    width: '100%',
    height: '100%',
    minHeight: TABLE_HEADER_MIN_HEIGHT,
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
  // Base suppression for all icons and native MRT badges until shown by the container above
  '&& .MuiBadge-root, && .ColumnActionsMenuButton': {
    display: 'none',
  },
  // Target the SORT badges specifically for visibility
  ['&& .fleet-column-actions.is-sorted .MuiBadge-root, ' +
  '&&:hover .fleet-column-actions .MuiBadge-root, ' +
  '&& .fleet-column-actions:focus-within .MuiBadge-root']: {
    display: 'inline-flex',
    marginRight: '4px', // Keep a small gap between the badge and the next icon
  },
  // Target the SORT strings specifically for circular hover
  ['&& .fleet-column-actions.is-sorted .MuiTableSortLabel-root, ' +
  '&&:hover .fleet-column-actions .MuiTableSortLabel-root, ' +
  '&& .fleet-column-actions:focus-within .MuiTableSortLabel-root']: {
    display: 'flex',
    borderRadius: radii.pill,
    padding: '0 4px',
    minWidth: '24px',
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
      fontSize: iconSizes.inline,
      opacity: '1 !important', // Ensure MUI always renders the icon when inside this container
    },
    // Give the hover effect a pill-like structure so the badge fits comfortably
    width: 'auto',
  },
  // We want the icon group (sort label) contiguous to other items, so keep a very small gap.
  '&& .fleet-column-actions .MuiBadge-root': {
    marginRight: '2px',
  },
  // The badge number should overlap the top right of the sort arrow like a small exponent notification badge.
  '&& .fleet-column-actions .MuiBadge-badge:not(.MuiBadge-invisible)': {
    transform: 'scale(1) translate(50%, -50%)',
    right: '4px',
    top: '6px',
    pointerEvents: 'none',
    width: '13px',
    height: '13px',
    minWidth: '13px',
    borderRadius: '50%',
    lineHeight: '13px',
    fontSize: '9px',
    padding: 0,
    backgroundColor: 'transparent',
    color: `${colors.grey[900]}`,
    fontWeight: 700,
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
      fontSize: `${iconSizes.inline}px !important`,
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
      fontSize: iconSizes.control, // Matches InfoTooltip fontSize override
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
    backgroundColor: `${fleetColors.border} !important`,
    borderWidth: '0 !important',
    border: 'none !important',
    opacity: '1 !important',
    visibility: 'visible !important',
    height: '18px !important',
    position: 'static !important',
    alignSelf: 'center !important',
    width: '1px !important',
    borderRadius: `${radii.pill} !important`,
    transition: 'all 0.2s',
    '&:hover, &[data-isresizing="true"]': {
      backgroundColor: `${fleetColors.primary} !important`,
      width: '3px !important',
      borderRadius: `${radii.pill} !important`,
    },
  },
  // MRT shifts the divider `translateX(4px)` so that it straddles the column
  // boundary. On the last column that pushes it 1-2px past the table's right
  // edge, which makes the table container's scrollWidth exceed its clientWidth
  // and shows a horizontal scrollbar even when every column fits. Keep the
  // last divider inside its cell instead.
  '&&:last-of-type .Mui-TableHeadCell-ResizeHandle-Divider': {
    transform: 'none',
  },
};

export const fleetTableBodyCellFlatSx: SxProps<Theme> = {
  fontFamily: fontFamilies.text,
  fontSize: fontSizes.label,
  color: fleetColors.textPrimary,
  fontVariantNumeric: 'tabular-nums',
  paddingTop: 'var(--cell-padding-vertical)',
  paddingBottom: 'var(--cell-padding-vertical)',
  paddingLeft: 'var(--cell-padding-horizontal)',
  paddingRight: 'var(--cell-padding-horizontal)',
  transition: 'padding 0.2s',
};

export const fleetTableBodyCellSx: SxProps<Theme> = {
  ...fleetTableBodyCellFlatSx,
  '&.select-col': {
    paddingLeft: SELECT_COL_PADDING,
    paddingRight: SELECT_COL_PADDING,
  },
  '&&.column-highlight': {
    backgroundColor: fleetColors.primaryContainer,
    color: fleetColors.primary,
  },
};
