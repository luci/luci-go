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

import { OptionsDropdown } from './options_dropdown';
import { FilterOption, SelectedFilters } from './types';

export function SelectedChip({
  option,
  selectedOptions,
  setSelectedOptions,
}: {
  option: FilterOption;
  filterOptions: FilterOption[]; // TODO(pietroscutta) remove
  selectedOptions: SelectedFilters;
  setSelectedOptions: React.Dispatch<React.SetStateAction<SelectedFilters>>;
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
              {`${option.options?.length} | [ ${option.label} ]: ${option?.options
                ?.filter((o2) => selectedOptions[option.value]?.[o2.value])
                ?.map((o2) => o2.label)
                .join(', ')} `}
            </span>
            <ArrowDropDownIcon />
          </p>
        }
        sx={{
          maxWidth: 300,
        }}
        color="primary"
        onDelete={(_) =>
          setSelectedOptions((currentFilters: SelectedFilters) => ({
            ...currentFilters,
            [option.value]: {},
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
      />
    </>
  );
}
