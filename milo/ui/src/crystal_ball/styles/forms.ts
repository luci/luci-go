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

import { SxProps, Theme } from '@mui/material/styles';

/**
 * Styles for a compact Select component.
 */
export const COMPACT_SELECT_SX: SxProps<Theme> = {
  minWidth: (theme) => theme.spacing(15),
  bgcolor: 'background.paper',
  height: (theme) => theme.spacing(4),
  '& .MuiSelect-select': {
    height: (theme) => theme.spacing(4),
    py: 0,
    display: 'flex',
    alignItems: 'center',
    fontSize: (theme) => theme.typography.body2.fontSize,
  },
};

/**
 * Styles for a compact TextField component.
 */
export const COMPACT_TEXTFIELD_SX: SxProps<Theme> = {
  bgcolor: 'background.paper',
  '& .MuiInputBase-root': { height: (theme) => theme.spacing(4) },
  '& .MuiInputBase-input': {
    fontSize: (theme) => theme.typography.body2.fontSize,
  },
};

/**
 * Styles for compressed icons used in compact forms.
 */
export const COMPACT_ICON_SX: SxProps<Theme> = {
  fontSize: (theme) => theme.typography.caption.fontSize,
  color: 'text.secondary',
};

/**
 * Styles for a compact filter row layout.
 * Columns: Column (2fr), Operator (1fr), Value (4fr), Delete button (auto).
 */
export const COMPACT_FILTER_ROW_SX: SxProps<Theme> = {
  display: 'grid',
  gridTemplateColumns: '2fr 1fr 4fr auto',
  gap: 1,
  alignItems: 'center',
  mb: 1.5,
};
