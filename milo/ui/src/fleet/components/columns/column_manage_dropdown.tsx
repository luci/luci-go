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

import { useFleetAnalytics } from '@/fleet/hooks/use_fleet_analytics';
import { OptionValue } from '@/fleet/types/option';
import { partition } from '@/fleet/utils/array';
import { fuzzySort } from '@/fleet/utils/fuzzy_sort';

import { MenuSkeleton } from '../filter_dropdown/menu_skeleton';
import { OptionsMenu } from '../filter_dropdown/options_menu';
import { OptionsDropdown } from '../options_dropdown';

import { ColumnOption } from './use_mrt_column_management';

interface ColumnsManageDropDownProps {
  isLoading?: boolean;
  anchorEl: HTMLElement | null;
  setAnchorEl: (newAnchorEl: HTMLElement | null) => void;
  onReset?: () => void;
  onResetWidths?: () => void;
  temporaryColumns?: string[];
  addUserVisibleColumn?: (column: string) => void;
  allColumns: ColumnOption[];
  visibleColumns: string[];
  onToggleColumn: (id: string) => void;
  selectOnlyColumn?: (id: string) => void;
}

/**
 * Column customization dropdown.
 */
export function ColumnsManageDropDown({
  isLoading,
  anchorEl,
  setAnchorEl,
  onReset,
  onResetWidths,
  temporaryColumns,
  allColumns,
  visibleColumns,
  onToggleColumn,
  selectOnlyColumn,
}: ColumnsManageDropDownProps) {
  const { trackEvent } = useFleetAnalytics();

  const selectedColumns = useMemo(() => {
    const selected = new Set(visibleColumns);
    for (const col of allColumns) {
      if (col.isTemporary) {
        selected.add(col.id);
      }
    }
    return selected;
  }, [visibleColumns, allColumns]);

  const effectiveTemporaryColumns = useMemo(() => {
    const temp = new Set(temporaryColumns ?? []);
    for (const col of allColumns) {
      if (col.isTemporary || col.isHighlighted) {
        temp.add(col.id);
      }
    }
    return temp;
  }, [temporaryColumns, allColumns]);

  const toggleColumn = (field: string) => {
    const isCurrentlyVisible = selectedColumns.has(field);
    trackEvent('column_toggled', {
      componentName: 'column_manage_dropdown',
      columnId: field,
      visible: !isCurrentlyVisible,
    });
    onToggleColumn(field);
  };

  const handleSelectOnlyColumn = selectOnlyColumn
    ? (field: string) => {
        trackEvent('column_select_only', {
          componentName: 'column_manage_dropdown',
          columnId: field,
          visible: true,
        });
        selectOnlyColumn(field);
      }
    : undefined;

  const columns = useMemo(
    () =>
      allColumns.map(
        (d) =>
          ({
            label: effectiveTemporaryColumns.has(d.id)
              ? `${d.label} *`
              : d.label,
            value: d.id,
          }) as OptionValue,
      ),
    [allColumns, effectiveTemporaryColumns],
  );

  const defaultSortedColumns = useMemo(
    () => fuzzySort('')(columns, (x: OptionValue) => x.label),
    [columns],
  );

  return (
    <OptionsDropdown
      sx={{ width: '500px' }}
      onClose={() => setAnchorEl(null)}
      anchorEl={anchorEl}
      open={!!anchorEl}
      anchorOrigin={{
        vertical: 'bottom',
        horizontal: 'right',
      }}
      enableSearchInput={true}
      maxHeight={500}
      onResetClick={onReset}
      onResetWidthsClick={onResetWidths}
      footerButtons={['reset', 'reset-widths']}
      onApply={() => {}}
      renderChild={(searchQuery) => {
        if (isLoading) {
          return <MenuSkeleton itemCount={columns.length} maxHeight={200} />;
        }

        const hasSearchQuery = searchQuery.trim().length > 0;

        const sortedColumns = hasSearchQuery
          ? fuzzySort(searchQuery)(
              columns,
              (x: OptionValue) => x.label,
              (x: OptionValue) => x.value,
            )
              .filter((item) => item.score > 0)
              .map((item) => ({
                ...item,
                el: {
                  ...item.el,
                  isSignificant: item.score > 0,
                },
              }))
          : defaultSortedColumns;

        const finalSortedColumns = hasSearchQuery
          ? sortedColumns
          : (() => {
              const [pass, fail] = partition(
                sortedColumns,
                (x) =>
                  selectedColumns.has(x.el.value) ||
                  effectiveTemporaryColumns.has(x.el.value),
              );
              return [...pass, ...fail];
            })();

        return (
          <OptionsMenu
            elements={finalSortedColumns}
            selectedElements={selectedColumns}
            flipOption={toggleColumn}
            selectOnly={handleSelectOnlyColumn}
          />
        );
      }}
    />
  );
}
