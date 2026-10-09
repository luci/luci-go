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

import { Add } from '@mui/icons-material';
import { ButtonBase, Typography } from '@mui/material';

export interface AddSliceCardProps {
  readonly isDragOver?: boolean;
  readonly onClick: () => void;
  readonly onDragOver?: (e: React.DragEvent) => void;
  readonly onDragLeave?: () => void;
  readonly onDrop?: (e: React.DragEvent) => void;
}

export const AddSliceCard = ({
  isDragOver = false,
  onClick,
  onDragOver,
  onDragLeave,
  onDrop,
}: AddSliceCardProps) => (
  <ButtonBase
    component="div"
    onClick={onClick}
    aria-label="Add Slice"
    data-testid="add-slice-card"
    onDragOver={onDragOver}
    onDragLeave={onDragLeave}
    onDrop={onDrop}
    sx={{
      position: 'relative',
      zIndex: isDragOver ? 10 : 1,
      p: 1.75,
      borderRadius: 2,
      border: '1px dashed',
      borderColor: isDragOver ? 'primary.main' : 'divider',
      outline: isDragOver ? '2px dashed #1976d2' : 'none',
      outlineOffset: isDragOver ? '2px' : 0,
      bgcolor: isDragOver ? 'rgba(25, 118, 210, 0.08)' : 'background.paper',
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      gap: 0.5,
      width: '100%',
      minHeight: 128,
      height: 128,
      boxSizing: 'border-box',
      cursor: 'pointer',
      transition: 'all 0.15s ease-in-out',
      '&:hover': {
        bgcolor: 'action.hover',
        borderColor: 'primary.main',
        boxShadow: '0 2px 6px rgba(0,0,0,0.06)',
        '& .add-icon': {
          color: 'primary.main',
        },
        '& .add-label': {
          color: 'primary.main',
        },
      },
    }}
  >
    <Add
      className="add-icon"
      sx={{
        fontSize: 24,
        color: 'text.secondary',
        transition: 'color 0.15s ease-in-out',
      }}
    />
    <Typography
      className="add-label"
      component="span"
      sx={{
        fontWeight: 600,
        fontSize: 13,
        color: 'text.secondary',
        transition: 'color 0.15s ease-in-out',
      }}
    >
      Add Slice
    </Typography>
  </ButtonBase>
);
