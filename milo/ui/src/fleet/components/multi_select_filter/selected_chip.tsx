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

import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown';
import { Chip } from '@mui/material';
import { useState } from 'react';

import { OptionCategory, SelectedOptions } from '@/fleet/types';

import { OptionsDropdown } from '../options_dropdown';

export function SelectedChip({
  option,
  selectedOptions,
  setSelectedOptions,
  isLoading,
}: {
  option: OptionCategory;
  selectedOptions: SelectedOptions;
  setSelectedOptions: React.Dispatch<React.SetStateAction<SelectedOptions>>;
  isLoading?: boolean;
}) {
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
              {`${selectedOptions[option.value].length} | [ ${option.label} ]: ${option?.options
                ?.filter((o2) =>
                  selectedOptions[option.value]?.includes(o2.value),
                )
                ?.map((o2) => o2.label)
                .join(', ')}`}
            </span>
            <ArrowDropDownIcon />
          </p>
        }
        sx={{
          maxWidth: 300,
        }}
        variant="filter"
        onDelete={(_) =>
          setSelectedOptions((currentFilters: SelectedOptions) => ({
            ...currentFilters,
            [option.value]: [],
          }))
        }
      />
      <OptionsDropdown
        onClose={() => setAnchorEL(null)}
        anchorEl={anchorEl}
        open={anchorEl !== null}
        option={option}
        selectedOptions={selectedOptions}
        setSelectedOptions={setSelectedOptions}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'left',
        }}
        isLoading={isLoading}
      />
    </>
  );
}
