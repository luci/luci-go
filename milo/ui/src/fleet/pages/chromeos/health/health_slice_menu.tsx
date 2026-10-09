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

import { DeleteOutline, Edit, PushPin } from '@mui/icons-material';
import {
  Divider,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
} from '@mui/material';

import { HealthSlice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export interface HealthSliceMenuProps {
  readonly slice: HealthSlice;
  readonly isDefault: boolean;
  readonly anchorEl: HTMLElement | null;
  readonly onClose: (e?: React.MouseEvent) => void;
  readonly onEdit: (slice: HealthSlice) => void;
  readonly onToggleDefault?: (slice: HealthSlice) => void;
  readonly onDelete?: (slice: HealthSlice) => void;
}

export const HealthSliceMenu = ({
  slice,
  isDefault,
  anchorEl,
  onClose,
  onEdit,
  onToggleDefault,
  onDelete,
}: HealthSliceMenuProps) => (
  <Menu
    anchorEl={anchorEl}
    open={Boolean(anchorEl)}
    onClose={(e: React.MouseEvent) => {
      e?.stopPropagation?.();
      onClose(e);
    }}
    onClick={(e) => e.stopPropagation()}
  >
    <MenuItem
      onClick={(e) => {
        e.stopPropagation();
        onClose(e);
        onEdit(slice);
      }}
    >
      <ListItemIcon>
        <Edit fontSize="small" />
      </ListItemIcon>
      <ListItemText primaryTypographyProps={{ fontSize: '0.8125rem' }}>
        Edit slice
      </ListItemText>
    </MenuItem>

    {onToggleDefault && (
      <MenuItem
        onClick={(e) => {
          e.stopPropagation();
          onClose(e);
          onToggleDefault(slice);
        }}
      >
        <ListItemIcon>
          <PushPin
            fontSize="small"
            sx={{
              color: isDefault ? 'primary.main' : 'inherit',
              transform: isDefault ? 'none' : 'rotate(45deg)',
            }}
          />
        </ListItemIcon>
        <ListItemText primaryTypographyProps={{ fontSize: '0.8125rem' }}>
          {isDefault ? 'Remove as default' : 'Set as default view'}
        </ListItemText>
      </MenuItem>
    )}

    {onDelete && <Divider />}

    {onDelete && (
      <MenuItem
        onClick={(e) => {
          e.stopPropagation();
          onClose(e);
          onDelete(slice);
        }}
        sx={{ color: 'error.main' }}
      >
        <ListItemIcon>
          <DeleteOutline fontSize="small" color="error" />
        </ListItemIcon>
        <ListItemText primaryTypographyProps={{ fontSize: '0.8125rem' }}>
          Delete slice
        </ListItemText>
      </MenuItem>
    )}
  </Menu>
);
