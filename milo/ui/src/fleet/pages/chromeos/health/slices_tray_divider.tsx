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

import { Add, ExpandMore } from '@mui/icons-material';
import { Button, Divider } from '@mui/material';

export interface SlicesTrayDividerProps {
  /** Total number of slices currently configured. */
  readonly slicesCount: number;
  /** Whether the tray is currently expanded. */
  readonly isExpanded: boolean;
  /** Callback fired when the user clicks the "Add Slice" divider button (when count === 4). */
  readonly onAddSlice: () => void;
  /** Callback fired when the user clicks the "All Slices (N)" divider button (when count >= 5). */
  readonly onExpand: () => void;
}

export const SlicesTrayDivider = ({
  slicesCount,
  isExpanded,
  onAddSlice,
  onExpand,
}: SlicesTrayDividerProps) => {
  // If expanded or fewer than 4 slices, no divider bar is needed.
  if (isExpanded || slicesCount < 4) {
    return null;
  }

  const isAddMode = slicesCount === 4;

  return (
    <Divider sx={{ mt: 1.5, borderColor: 'divider' }}>
      {isAddMode ? (
        <Button
          size="small"
          onClick={onAddSlice}
          startIcon={<Add sx={{ fontSize: 16 }} />}
          data-testid="add-slice-divider-button"
          sx={{
            textTransform: 'none',
            fontSize: 12,
            fontWeight: 600,
            color: 'primary.main',
            bgcolor: 'background.paper',
            px: 1.5,
            py: 0.25,
            borderRadius: 1,
            border: 'none',
            boxShadow: 'none',
            '&:hover': {
              bgcolor: 'action.hover',
              color: 'primary.dark',
            },
          }}
        >
          Add Slice
        </Button>
      ) : (
        <Button
          size="small"
          onClick={onExpand}
          endIcon={<ExpandMore sx={{ fontSize: 16 }} />}
          data-testid="expand-slices-button"
          sx={{
            textTransform: 'none',
            fontSize: 12,
            fontWeight: 600,
            color: 'primary.main',
            bgcolor: 'background.paper',
            px: 1.5,
            py: 0.25,
            borderRadius: 1,
            border: 'none',
            boxShadow: 'none',
            '&:hover': {
              bgcolor: 'action.hover',
              color: 'primary.dark',
            },
          }}
        >
          All Slices ({slicesCount})
        </Button>
      )}
    </Divider>
  );
};
