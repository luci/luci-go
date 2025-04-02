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

import AddIcon from '@mui/icons-material/Add';
import { Chip, colors } from '@mui/material';
import { useState } from 'react';

import {
  FilterDropdown,
  FilterCategoryData as FilterCategoryData,
} from './filter_dropdown';

export function FilterButton<T>({
  filterOptions,
  isLoading,
  onApply,
  commonOptions,
}: {
  filterOptions: FilterCategoryData<T>[];
  isLoading?: boolean;
  onApply: () => void;
  commonOptions?: string[];
}) {
  const [anchorEl, setAnchorEL] = useState<HTMLElement | null>(null);

  return (
    <>
      <Chip
        onClick={(event) => setAnchorEL(event.currentTarget)}
        label="Add filter"
        variant="outlined"
        icon={<AddIcon sx={{ color: colors.blue[600], width: 18 }} />}
      ></Chip>
      <FilterDropdown
        filterOptions={filterOptions}
        anchorEl={anchorEl}
        setAnchorEL={setAnchorEL}
        isLoading={isLoading}
        onApply={onApply}
        commonOptions={commonOptions}
      />
    </>
  );
}
