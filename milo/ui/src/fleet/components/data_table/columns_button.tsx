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

import Button from '@mui/material/Button';
import {
  gridColumnDefinitionsSelector,
  GridColumnIcon,
  gridColumnVisibilityModelSelector,
} from '@mui/x-data-grid';
import { GridApiCommunity } from '@mui/x-data-grid/internals';
import { useState } from 'react';

import { Option, SelectedOptions } from '@/fleet/types';

import { OptionsDropdown } from '../options_dropdown';

interface ColumnsButtonProps {
  gridRef: React.MutableRefObject<GridApiCommunity>;
}

/**
 * Component for displaying a menu to customize the columns on a table.
 */
export function ColumnsButton({ gridRef }: ColumnsButtonProps) {
  const columnVisibilityModel = gridColumnVisibilityModelSelector(gridRef);
  const columnDefinitions = gridColumnDefinitionsSelector(gridRef);

  const [anchorEl, setAnchorEL] = useState<HTMLElement | null>(null);

  const toggleColumn = (field: string) => {
    if (!columnVisibilityModel) {
      return;
    }

    gridRef.current.setColumnVisibility(field, !columnVisibilityModel[field]);
  };

  const columns: Option = {
    label: 'column',
    value: 'column',
    options: columnDefinitions.map((column) => ({
      label: column.headerName ?? column.field,
      value: column.field,
    })),
  };

  const selectedColumns: SelectedOptions = {
    column: columnVisibilityModel
      ? Object.keys(columnVisibilityModel).filter(
          (key) => columnVisibilityModel[key],
        )
      : [],
  };

  return (
    <>
      <Button
        onClick={(event) => setAnchorEL(event.currentTarget)}
        size="small"
        startIcon={<GridColumnIcon />}
      >
        Columns
      </Button>
      <OptionsDropdown
        onClose={() => setAnchorEL(null)}
        selectedOptions={selectedColumns}
        anchorEl={anchorEl}
        open={!!anchorEl}
        option={columns}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'center',
        }}
        disableFooter={true}
        enableSearchInput={true}
        onFlipOption={toggleColumn}
        maxHeight={500}
      />
    </>
  );
}
