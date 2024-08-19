// Copyright 2024 The LUCI Authors.
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
  TextField,
  MenuItem,
  FormControl,
  Select,
  SelectChangeEvent,
  Box,
  InputLabel,
  SxProps,
  Theme,
} from '@mui/material';
import { ChangeEvent } from 'react';

interface SelectTextFieldProps {
  label: string;
  selectValue: string;
  textValue: string;
  options: string[];
  selectOnChange: (value: string) => void;
  textOnChange: (value: string) => void;
  sx?: SxProps<Theme>;
  required?: boolean;
}

// SelectTextField is a component that combines a dropdown field with a text input field.
export function SelectTextField({
  sx,
  label,
  selectValue,
  textValue,
  options,
  selectOnChange,
  textOnChange,
  required = false,
}: SelectTextFieldProps) {
  const handleTextChange = (event: ChangeEvent<HTMLInputElement>) => {
    textOnChange(event.target.value);
  };

  const handleSelectChange = (event: SelectChangeEvent<string>) => {
    selectOnChange(event.target.value);
  };

  const unspecified = required && textValue === '';
  return (
    <Box sx={{ flex: 1, ...sx }}>
      <Box display="flex" alignItems="center">
        <FormControl variant="outlined" size="small" sx={{ minWidth: '140px' }}>
          <InputLabel id="select-label">
            {label} {required && '*'}
          </InputLabel>
          <Select
            value={selectValue}
            labelId="select-label"
            label={label}
            onChange={handleSelectChange}
            size="small"
            sx={{ borderRadius: '4px 0px 0px 4px' }}
            inputProps={{
              'data-testid': label + ' select',
            }}
          >
            {options.map((option) => (
              <MenuItem key={option} value={option}>
                {option}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <TextField
          onChange={handleTextChange}
          fullWidth
          value={textValue}
          variant="outlined"
          size="small"
          sx={{
            ml: 0,
            flex: 1,
            '& .MuiInputBase-root': {
              borderRadius: '0px 4px 4px 0px',
              borderLeft: '0',
            },
            '& .MuiOutlinedInput-notchedOutline': {
              borderLeftColor: 'rgba(0, 0, 0, 0.0)',
            },
          }}
          inputProps={{
            'data-testid': label + ' input',
          }}
        />
      </Box>
      {unspecified && (
        <Box sx={{ color: '#d23a2d', paddingLeft: '15px' }}>
          {label} is required
        </Box>
      )}
    </Box>
  );
}
