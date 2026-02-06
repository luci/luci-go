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

import { useMemo } from 'react';

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
  allColumns: { id: string; label: string }[];
  visibleColumns: string[];
  onToggleColumn: (id: string) => void;
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
  allColumns,
  visibleColumns,
  onToggleColumn,
}: ColumnsButtonProps) {
  const toggleColumn = (field: string) => {
    if (temporaryColumns?.includes(field)) {
      // Toggling a temporary column makes it permanent.
      addUserVisibleColumn?.(field);
      return;
    }
    onToggleColumn(field);
  };

  const columns = useMemo(
    () =>
      allColumns.map(
        (d) =>
          ({
            label: temporaryColumns?.includes(d.id) ? `${d.label} *` : d.label,
            value: d.id,
          }) as OptionValue,
      ),
    [allColumns, temporaryColumns],
  );

  const defaultSortedColumns = useMemo(
    () => fuzzySort('')(columns, (x: OptionValue) => x.label),
    [columns],
  );

  const selectedColumns = visibleColumns.filter(
    (key) => !temporaryColumns?.includes(key),
  );

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

        const sortedColumns = searchQuery
          ? fuzzySort(searchQuery)(columns, (x: OptionValue) => x.label)
          : defaultSortedColumns;
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
