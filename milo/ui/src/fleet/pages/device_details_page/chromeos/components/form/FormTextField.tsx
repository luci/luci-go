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

import { Box, Chip } from '@mui/material';
import { useEffect, useState } from 'react';

import { PropertyField } from '../common/PropertyField';

import { useCardForm } from './CardForm';
import { useInventoryForm } from './InventoryFormContext';

interface FormTextFieldProps {
  label?: string;
  path: string | string[];
  type?: 'string' | 'number' | 'array';
  gridSm?: number;
  gridMd?: number;
}

export const FormTextField = ({
  label,
  path,
  type,
  gridSm = 6,
  gridMd,
}: FormTextFieldProps) => {
  const resolvedLabel = label ?? '';
  const resolvedType = type ?? 'string';

  const { isEditing, getFieldValue, setFieldValue, confirm } = useCardForm();
  const { isPathEditable } = useInventoryForm();
  const value = getFieldValue(path);

  const pathStr = typeof path === 'string' ? path : path.join('.');
  const isEditable = isPathEditable(pathStr);
  const shouldRenderEditMode = isEditing && isEditable;

  const [localVal, setLocalVal] = useState('');
  const [wasEditing, setWasEditing] = useState(false);

  useEffect(() => {
    // Only initialize local value when transitioning into edit mode.
    // Do NOT sync changes while editing is active, as updating localVal directly from value
    // would overwrite the user's cursor position and strip transient inputs
    // (e.g. typing commas in arrays or negative signs in numbers before completion).
    if (shouldRenderEditMode && !wasEditing) {
      const displayValue =
        resolvedType === 'array' && Array.isArray(value)
          ? value.join(',')
          : String(value ?? '');
      setLocalVal(displayValue);
    }
    setWasEditing(shouldRenderEditMode);
  }, [shouldRenderEditMode, wasEditing, value, resolvedType]);

  if (!shouldRenderEditMode) {
    if (resolvedType === 'array') {
      const arr = Array.isArray(value) ? value : [];
      if (arr.length === 0) return null;
      return (
        <PropertyField label={resolvedLabel} gridSm={gridSm} gridMd={gridMd}>
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.75 }}>
            {arr.map((val, idx) => (
              <Chip
                key={idx}
                label={String(val)}
                size="small"
                color="primary"
                variant="outlined"
                sx={{ fontWeight: 600 }}
              />
            ))}
          </Box>
        </PropertyField>
      );
    }
    const hasValue = value !== undefined && value !== null && value !== '';
    if (!hasValue) return null;

    return (
      <PropertyField
        label={resolvedLabel}
        value={String(value)}
        gridSm={gridSm}
        gridMd={gridMd}
      />
    );
  }

  const handleChange = (newVal: string) => {
    setLocalVal(newVal);

    if (resolvedType === 'array') {
      const parsed = newVal
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean);
      setFieldValue(path, parsed);
    } else if (resolvedType === 'number') {
      if (newVal === '') {
        setFieldValue(path, null);
      } else {
        const num = Number(newVal);
        setFieldValue(path, isNaN(num) ? null : num);
      }
    } else {
      setFieldValue(path, newVal);
    }
  };

  return (
    <PropertyField
      label={resolvedLabel}
      value={localVal}
      editable={true}
      isEditing={true}
      onChange={handleChange}
      onConfirm={confirm}
      gridSm={gridSm}
      gridMd={gridMd}
      inputType={resolvedType === 'number' ? 'number' : 'text'}
    />
  );
};
