// Copyright 2025 The LUCI Authors.
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

import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown';
import { Chip } from '@mui/material';
import { ReactNode, useState } from 'react';

import { OptionsDropdown } from '../options_dropdown/options_dropdown';

interface SelectedChipProps {
  dropdownContent: (searchQuery: string) => ReactNode;
  label: string;
  onApply: () => void;
  onDelete: () => void;
}

export function SelectedChip({
  dropdownContent,
  label,
  onApply,
  onDelete,
}: SelectedChipProps) {
  const [anchorEl, setAnchorEL] = useState<HTMLElement | null>(null);

  return (
    <>
      <Chip
        onClick={(event) => setAnchorEL(event.currentTarget)}
        label={
          <p style={{ display: 'flex', alignItems: 'center' }}>
            <span
              style={{
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
            >
              {label}
            </span>
            <ArrowDropDownIcon />
          </p>
        }
        sx={{
          maxWidth: 300,
        }}
        variant="filter"
        onDelete={onDelete}
      />
      <OptionsDropdown
        anchorEl={anchorEl}
        open={!!anchorEl}
        enableSearchInput
        renderChild={dropdownContent}
        onApply={() => {
          setAnchorEL(null);
          onApply();
        }}
        onClose={() => setAnchorEL(null)}
      />
    </>
  );
}
