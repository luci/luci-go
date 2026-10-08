// Copyright 2026 The LUCI Authors.
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

import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import {
  Box,
  Button,
  CircularProgress,
  IconButton,
  TextField,
} from '@mui/material';
import { useCallback, useMemo } from 'react';

import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import {
  FilterCategory,
  FilterCategoryBuilder,
  useFilterState,
} from '@/fleet/components/filters/use_filters';

import {
  EditableRuleRow,
  isRowDirty,
  MAX_RULE_WEIGHT,
  MIN_RULE_WEIGHT,
} from './editable_rule_row';

export interface PriorityRuleRowProps {
  readonly row: EditableRuleRow;
  readonly index: number;
  readonly filterBuilders: Record<
    string,
    FilterCategoryBuilder<FilterCategory>
  >;
  readonly isBuildersLoading: boolean;
  readonly isBusy: boolean;
  readonly isSubmitting: boolean;
  readonly canEdit: boolean;
  /** Placeholder shown in the rule filter input while editing. */
  readonly searchPlaceholder?: string;
  readonly onFilterChange: (id: string, nextAip160: string) => void;
  readonly onWeightChange: (id: string, weight: string) => void;
  readonly onApply: (row: EditableRuleRow) => void;
  readonly onDelete: (row: EditableRuleRow) => void;
}

export const PriorityRuleRow = ({
  row,
  index,
  filterBuilders,
  isBuildersLoading,
  isBusy,
  isSubmitting,
  canEdit,
  searchPlaceholder = 'Add rule filter...',
  onFilterChange,
  onWeightChange,
  onApply,
  onDelete,
}: PriorityRuleRowProps) => {
  const handleFilterChange = useCallback(
    (nextAip160: string) => {
      if (!canEdit) return;
      onFilterChange(row.id, nextAip160);
    },
    [canEdit, onFilterChange, row.id],
  );

  const { filterValues } = useFilterState(
    filterBuilders,
    row.expressionAip160,
    handleFilterChange,
    {
      areFilterValuesLoading: isBuildersLoading,
    },
  );

  const filterCategoryDatas = useMemo(
    () => (filterValues ? Object.values(filterValues) : []),
    [filterValues],
  );

  const dirty = isRowDirty(row);

  return (
    <Box
      data-testid={`priority-rule-row-${row.id}`}
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 1.5,
        width: '100%',
      }}
    >
      <Box
        inert={!canEdit}
        sx={{
          flex: 1,
          minWidth: 0,
          ...(!canEdit && {
            pointerEvents: 'none',
            '& .MuiChip-deleteIcon': {
              display: 'none',
            },
            '& .MuiSvgIcon-root': {
              display: 'none',
            },
            '& input': {
              display: 'none',
            },
          }),
        }}
      >
        <FilterBar
          filterCategoryDatas={filterCategoryDatas}
          isLoading={isBuildersLoading}
          searchPlaceholder={canEdit ? searchPlaceholder : ''}
          disableShortcut
        />
      </Box>

      <TextField
        label="Pts"
        type="number"
        value={row.weight}
        onChange={(e) => onWeightChange(row.id, e.target.value)}
        onKeyDown={(e) => {
          if (canEdit && e.key === 'Enter' && !isSubmitting) {
            e.preventDefault();
            onApply(row);
          }
        }}
        size="small"
        disabled={!canEdit || isBusy}
        sx={{
          width: 100,
          flexShrink: 0,
          ...(!canEdit && {
            '& .MuiInputBase-input.Mui-disabled': {
              WebkitTextFillColor: (theme) => theme.palette.text.primary,
              color: 'text.primary',
            },
            '& .MuiInputLabel-root.Mui-disabled': {
              color: 'text.primary',
            },
            '& .MuiOutlinedInput-root.Mui-disabled .MuiOutlinedInput-notchedOutline':
              {
                borderColor: 'rgba(0, 0, 0, 0.23)',
              },
          }),
        }}
        inputProps={{
          readOnly: !canEdit,
          min: MIN_RULE_WEIGHT,
          max: MAX_RULE_WEIGHT,
          'data-testid': `rule-weight-input-${row.id}`,
          'aria-label': `Rule ${index + 1} points weight`,
        }}
      />

      {canEdit && dirty && (
        <Button
          variant="contained"
          color="primary"
          size="small"
          onClick={() => onApply(row)}
          disabled={isSubmitting}
          data-testid={`rule-apply-button-${row.id}`}
          sx={{ minWidth: 64, height: 40, flexShrink: 0 }}
        >
          {isBusy ? <CircularProgress size={20} color="inherit" /> : 'Apply'}
        </Button>
      )}

      {canEdit && (
        <IconButton
          aria-label={`delete rule ${row.id}`}
          size="small"
          onClick={() => onDelete(row)}
          disabled={isSubmitting}
          data-testid={`rule-delete-button-${row.id}`}
          sx={{ color: '#d32f2f', flexShrink: 0 }}
        >
          <DeleteOutlineIcon fontSize="small" />
        </IconButton>
      )}
    </Box>
  );
};
