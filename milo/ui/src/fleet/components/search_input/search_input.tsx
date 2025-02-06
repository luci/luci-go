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

import SearchIcon from '@mui/icons-material/Search';
import { TextField, TextFieldProps } from '@mui/material';
import _ from 'lodash';

export type SearchInputProps = TextFieldProps & {
  searchInput: React.RefObject<HTMLInputElement>;
  searchQuery: string;
  onChange: (event: React.ChangeEvent<HTMLInputElement>) => void;
};

export function SearchInput({
  searchInput,
  searchQuery,
  onChange,
  ...textFieldProps
}: SearchInputProps) {
  return (
    <div
      role="menu"
      tabIndex={0}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 15,
        padding: '0 10px',
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          searchInput.current?.focus();
        }
      }}
    >
      <TextField
        inputRef={searchInput}
        placeholder="search"
        variant="standard"
        onChange={onChange}
        value={searchQuery}
        fullWidth
        // eslint-disable-next-line jsx-a11y/no-autofocus
        autoFocus
        onKeyDown={(e) => {
          e.stopPropagation();
          if (
            e.key === 'Escape' ||
            e.key === 'ArrowDown' ||
            (e.key === 'j' && e.ctrlKey)
          ) {
            e.currentTarget.parentElement?.focus();
            e.preventDefault();
          }
        }}
        slotProps={{
          input: {
            startAdornment: <SearchIcon sx={{ marginRight: '10px' }} />,
          },
        }}
        {...textFieldProps}
      />
    </div>
  );
}
