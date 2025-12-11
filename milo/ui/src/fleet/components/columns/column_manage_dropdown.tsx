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
  GridColDef,
  gridColumnDefinitionsSelector,
  gridColumnVisibilityModelSelector,
  useGridApiContext,
} from '@mui/x-data-grid';

import { OptionsDropdown } from '@/fleet/components/options_dropdown';
import { OptionValue } from '@/fleet/types/option';
import { fuzzySort } from '@/fleet/utils/fuzzy_sort';

import { MenuSkeleton } from '../filter_dropdown/menu_skeleton';
import { OptionsMenu } from '../filter_dropdown/options_menu';

interface ColumnsButtonProps {
  isLoading?: boolean;
  anchorEl: HTMLElement | null;
  setAnchorEL: (newAnchorEl: HTMLElement | null) => void;
  onReset?: () => void;
  temporaryColumns?: string[];
  addUserVisibleColumn?: (column: string) => void;
}

/**
 * Column customization dropdown.
 */
export function ColumnsManageDropDown({
  isLoading,
  anchorEl,
  setAnchorEL,
  onReset,
  temporaryColumns,
  addUserVisibleColumn,
}: ColumnsButtonProps) {
  const apiRef = useGridApiContext();
  const columnVisibilityModel = gridColumnVisibilityModelSelector(apiRef);
  const columnDefinitions = gridColumnDefinitionsSelector(apiRef);

  const toggleColumn = (field: string) => {
    if (!columnVisibilityModel) {
      return;
    }
    if (temporaryColumns?.includes(field)) {
      // Toggling a temporary column makes it permanent.
      addUserVisibleColumn?.(field);
      return;
    }

    apiRef.current.setColumnVisibility(field, !columnVisibilityModel[field]);
  };

  const renderLabel = (col: GridColDef) => {
    const label = col.headerName || col.field;
    return temporaryColumns?.includes(col.field) ? `${label} *` : label;
  };

  const columns = columnDefinitions
    .filter((column) => column.field !== '__check__')
    .map(
      (d) =>
        ({
          label: renderLabel(d),
          value: d.field,
        }) as OptionValue,
    );

  const selectedColumns = columnVisibilityModel
    ? Object.keys(columnVisibilityModel).filter(
        (key) => columnVisibilityModel[key] && !temporaryColumns?.includes(key),
      )
    : [];

  return (
    <OptionsDropdown
      onClose={() => setAnchorEL(null)}
      anchorEl={anchorEl}
      open={!!anchorEl}
      anchorOrigin={{
        vertical: 'bottom',
        horizontal: 'center',
      }}
      enableSearchInput={true}
      maxHeight={500}
      onResetClick={onReset}
      footerButtons={['reset']}
      onApply={() => {}}
      renderChild={(searchQuery) => {
        if (isLoading) {
          return <MenuSkeleton itemCount={columns.length} maxHeight={200} />;
        }

        const sortedColumns = fuzzySort(searchQuery)(columns, (x) => x.label);
        return (
          <OptionsMenu
            elements={sortedColumns}
            selectedElements={new Set(selectedColumns)}
            flipOption={toggleColumn}
          />
        );
      }}
    />
  );
}
