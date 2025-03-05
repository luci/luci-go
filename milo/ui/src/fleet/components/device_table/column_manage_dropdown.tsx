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

import {
  gridColumnDefinitionsSelector,
  gridColumnVisibilityModelSelector,
  useGridApiContext,
} from '@mui/x-data-grid';

import { OptionCategory, SelectedOptions } from '@/fleet/types';

import { OptionsDropdown } from '../options_dropdown';

interface ColumnsButtonProps {
  isLoading?: boolean;
  anchorEl: HTMLElement | null;
  setAnchorEL: (newAnchorEl: HTMLElement | null) => void;
}

/**
 * Column customization dropdown.
 */
export function ColumnsManageDropDown({
  isLoading,
  anchorEl,
  setAnchorEL,
}: ColumnsButtonProps) {
  const apiRef = useGridApiContext();
  const columnVisibilityModel = gridColumnVisibilityModelSelector(apiRef);
  const columnDefinitions = gridColumnDefinitionsSelector(apiRef);

  const toggleColumn = (field: string) => {
    if (!columnVisibilityModel) {
      return;
    }

    apiRef.current.setColumnVisibility(field, !columnVisibilityModel[field]);
  };

  const columns: OptionCategory = {
    label: 'column',
    value: 'column',
    options: columnDefinitions
      .filter((column) => column.field !== '__check__')
      .map((column) => ({
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
      isLoading={isLoading}
    />
  );
}
