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

import { Grid, TextField, Typography } from '@mui/material';
import { ReactNode } from 'react';

import { CodeChip } from './CodeChip';

export interface PropertyFieldProps {
  label: string;
  value?: string | number | null;
  variant?: 'codeChip' | 'text';
  gridSm?: number;
  gridMd?: number;
  children?: ReactNode;
  editable?: boolean;
  isEditing?: boolean;
  onChange?: (newValue: string) => void;
  onConfirm?: () => void;
  inputType?: 'text' | 'number';
}

export const PropertyField = ({
  label,
  value,
  variant = 'codeChip',
  gridSm = 6,
  gridMd,
  children,
  editable = false,
  isEditing = false,
  onChange,
  onConfirm,
  inputType = 'text',
}: PropertyFieldProps) => {
  if (editable && isEditing) {
    return (
      <Grid item xs={12} sm={gridSm} md={gridMd}>
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ display: 'block', mb: 0.5 }}
        >
          {label}
        </Typography>
        <TextField
          type={inputType}
          value={value !== null && value !== undefined ? String(value) : ''}
          onChange={(e) => onChange?.(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              onConfirm?.();
            }
          }}
          size="small"
          variant="outlined"
          fullWidth
        />
      </Grid>
    );
  }

  const hasValue = value !== undefined && value !== null && value !== '';
  if (!hasValue && !children) return null;

  return (
    <Grid item xs={12} sm={gridSm} md={gridMd}>
      <Typography
        variant="caption"
        color="text.secondary"
        sx={{ display: 'block', mb: 0.5 }}
      >
        {label}
      </Typography>
      {children ??
        (variant === 'codeChip' ? (
          <CodeChip value={String(value)} />
        ) : (
          <Typography variant="body2">{value}</Typography>
        ))}
    </Grid>
  );
};
