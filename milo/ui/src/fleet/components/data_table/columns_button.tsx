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
  useGridApiContext,
  useGridSelector,
} from '@mui/x-data-grid';
import { useState } from 'react';

import { Option, SelectedOptions } from '@/fleet/types';

import { OptionsDropdown } from '../options_dropdown';

export function ColumnsButton() {
  const apiRef = useGridApiContext();
  const columnVisibilityModel = useGridSelector(
    apiRef,
    gridColumnVisibilityModelSelector,
  );
  const columnDefinitions = useGridSelector(
    apiRef,
    gridColumnDefinitionsSelector,
  );
  const [anchorEl, setAnchorEL] = useState<HTMLElement | null>(null);

  const toggleColumn = (field: string) => {
    apiRef.current.setColumnVisibility(field, !columnVisibilityModel[field]);
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
    column: Object.keys(columnVisibilityModel).filter(
      (key) => columnVisibilityModel[key],
    ),
  };

  return (
    <div>
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
    </div>
  );
}
