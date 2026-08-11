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

import {
  Autocomplete,
  Box,
  Chip,
  Grid,
  TextField,
  Typography,
  createFilterOptions,
} from '@mui/material';
import { useMemo } from 'react';
import { useEffect } from 'react';

import { CodeChip } from '../common/CodeChip';

import { useCardForm } from './CardForm';
import { useInventoryForm } from './InventoryFormContext';

interface FormAutocompleteFieldProps {
  label: string;
  path: string | string[];
  options?: string[];
  limitTags?: number;
  gridSm?: number;
  regexValidation?: RegExp;
  maxLength?: number;
  multiple?: boolean;
  freeSolo?: boolean;
  valueMapping?: {
    toStored: (displayVal: string) => unknown;
    toDisplay: (storedVal: unknown) => string;
  };
}

const filterOptionsLimit = createFilterOptions<string>({
  limit: 100,
});

export const FormAutocompleteField = ({
  label,
  path,
  options = [],
  limitTags = 5,
  gridSm = 6,
  regexValidation = /^[a-zA-Z0-9-_.]+$/,
  maxLength = 100,
  multiple = true,
  freeSolo = true,
  valueMapping,
}: FormAutocompleteFieldProps) => {
  const { isEditing, getFieldValue, setFieldValue, setFieldError } =
    useCardForm();
  const { isPathEditable } = useInventoryForm();

  const pathStr = typeof path === 'string' ? path : path.join('.');
  const isEditable = isPathEditable(pathStr);
  const value = getFieldValue(path);
  const arrValue = useMemo(() => {
    if (Array.isArray(value)) {
      return value.map((val) =>
        valueMapping ? valueMapping.toDisplay(val) : String(val),
      );
    }
    return [];
  }, [value, valueMapping]);

  const strValue = useMemo(() => {
    if (value !== undefined && value !== null) {
      return valueMapping ? valueMapping.toDisplay(value) : String(value);
    }
    return '';
  }, [value, valueMapping]);

  const tokensToValidate = multiple ? arrValue : strValue ? [strValue] : [];
  const invalidTokens = tokensToValidate.filter(
    (val) =>
      (regexValidation && !regexValidation.test(val)) || val.length > maxLength,
  );
  const hasError = invalidTokens.length > 0;

  useEffect(() => {
    if (isEditing && isEditable) {
      setFieldError(pathStr, hasError);
    }
    return () => {
      setFieldError(pathStr, false);
    };
  }, [hasError, pathStr, isEditing, isEditable, setFieldError]);

  if (!isEditing || !isEditable) {
    if (multiple) {
      if (arrValue.length === 0) return null;
      return (
        <Grid item xs={12} sm={gridSm}>
          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ display: 'block', mb: 0.5 }}
          >
            {label}
          </Typography>
          <Box
            sx={{
              display: 'flex',
              flexWrap: 'wrap',
              gap: 0.5,
            }}
          >
            {arrValue.map((val, idx) => (
              <Chip
                key={idx}
                label={val}
                size="small"
                color="primary"
                variant="outlined"
                sx={{ fontWeight: 600 }}
              />
            ))}
          </Box>
        </Grid>
      );
    }

    if (!strValue) return null;
    return (
      <Grid item xs={12} sm={gridSm}>
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ display: 'block', mb: 0.5 }}
        >
          {label}
        </Typography>
        <CodeChip value={strValue} />
      </Grid>
    );
  }

  const helperText = hasError
    ? `Invalid format or length: ${invalidTokens.join(', ')}`
    : undefined;

  if (!multiple) {
    return (
      <Grid item xs={12} sm={gridSm}>
        <Autocomplete
          freeSolo={freeSolo}
          size="small"
          options={options}
          filterOptions={filterOptionsLimit}
          value={strValue || null}
          onChange={(_, newValue) => {
            const val =
              typeof newValue === 'string' ? newValue : (newValue ?? '');
            setFieldValue(
              path,
              valueMapping ? valueMapping.toStored(val) : val,
            );
          }}
          onInputChange={(_, newInputValue, reason) => {
            if (freeSolo && (reason === 'input' || reason === 'clear')) {
              const val = newInputValue ?? '';
              setFieldValue(
                path,
                valueMapping ? valueMapping.toStored(val) : val,
              );
            }
          }}
          renderInput={(params) => (
            <TextField
              {...params}
              label={label}
              size="small"
              variant="outlined"
              fullWidth
              error={hasError}
              helperText={helperText}
            />
          )}
        />
      </Grid>
    );
  }

  return (
    <Grid item xs={12} sm={gridSm}>
      <Autocomplete
        multiple
        freeSolo={freeSolo}
        size="small"
        limitTags={limitTags}
        options={options}
        filterOptions={filterOptionsLimit}
        value={arrValue}
        onChange={(_, newValue) => {
          const vals = Array.isArray(newValue) ? newValue : [];
          setFieldValue(
            path,
            valueMapping ? vals.map((v) => valueMapping.toStored(v)) : vals,
          );
        }}
        renderTags={(value: readonly string[], getTagProps) =>
          value.map((option: string, index: number) => {
            const { key, ...tagProps } = getTagProps({ index });
            return (
              <Chip
                key={key}
                label={option}
                size="small"
                color="primary"
                variant="outlined"
                sx={{ fontWeight: 600 }}
                {...tagProps}
              />
            );
          })
        }
        renderInput={(params) => (
          <TextField
            {...params}
            label={label}
            size="small"
            variant="outlined"
            fullWidth
            error={hasError}
            helperText={helperText}
          />
        )}
      />
    </Grid>
  );
};
