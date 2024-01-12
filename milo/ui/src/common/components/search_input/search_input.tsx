// Copyright 2023 The LUCI Authors.
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

import Search from '@mui/icons-material/Search';
import Box from '@mui/material/Box';
import FormControl from '@mui/material/FormControl';
import InputAdornment from '@mui/material/InputAdornment';
import TextField from '@mui/material/TextField';
import { ChangeEventHandler } from 'react';

interface Props {
  placeholder: string;
  value: string;
  autoFocus?: boolean;
  onInputChange: ChangeEventHandler<HTMLTextAreaElement | HTMLInputElement>;
}

export const SearchInput = ({
  placeholder,
  value,
  autoFocus,
  onInputChange,
}: Props) => {
  return (
    <Box sx={{ display: 'grid', mx: 20, gridTemplateColumns: '1fr' }}>
      <FormControl>
        <TextField
          value={value}
          placeholder={placeholder}
          onChange={onInputChange}
          // Let the caller decide whether `autoFocus` should be used or not.
          // eslint-disable-next-line jsx-a11y/no-autofocus
          autoFocus={autoFocus}
          variant="outlined"
          size="small"
          inputProps={{
            'data-testid': 'filter-input',
          }}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <Search />
              </InputAdornment>
            ),
          }}
          sx={{
            marginLeft: '-1px',
            '& .MuiOutlinedInput-notchedOutline': {
              borderTopLeftRadius: 0,
              borderBottomLeftRadius: 0,
            },
          }}
        />
      </FormControl>
    </Box>
  );
};
