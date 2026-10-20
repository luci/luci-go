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
  ClickAwayListener,
  IconButton,
  TextField,
} from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';

import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import {
  FilterCategory,
  FilterCategoryBuilder,
  useFilterState,
} from '@/fleet/components/filters/use_filters';
import { fleetColors } from '@/fleet/theme/tokens';

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
  readonly onCancel: (row: EditableRuleRow) => void;
}

const cancelButtonSx = {
  minWidth: 64,
  height: 40,
  flexShrink: 0,
  color: 'text.secondary',
  borderColor: fleetColors.border,
  '&:hover': {
    borderColor: fleetColors.borderEmphasis,
    backgroundColor: fleetColors.surfaceTonal,
  },
} as const;

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
  onCancel,
}: PriorityRuleRowProps) => {
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

  useEffect(() => {
    if (!showDeleteConfirm) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !isSubmitting) {
        setShowDeleteConfirm(false);
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [showDeleteConfirm, isSubmitting]);

  const handleFilterChange = useCallback(
    (nextAip160: string) => {
      if (!canEdit) return;
      setShowDeleteConfirm(false);
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
  const isFilterEmpty = row.expressionAip160.trim() === '';
  const isEmptiedSavedRule = !row.isDraft && isFilterEmpty;
  const showInlineDelete =
    !row.isDraft && (isEmptiedSavedRule || showDeleteConfirm);

  const handleCancel = () => {
    if (showDeleteConfirm) {
      setShowDeleteConfirm(false);
    } else {
      onCancel(row);
    }
  };

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
        onChange={(e) => {
          setShowDeleteConfirm(false);
          onWeightChange(row.id, e.target.value);
        }}
        onKeyDown={(e) => {
          if (
            canEdit &&
            e.key === 'Enter' &&
            !isSubmitting &&
            !showInlineDelete
          ) {
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

      {canEdit && dirty && !showInlineDelete && (
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 1,
            flexShrink: 0,
          }}
        >
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
          <Button
            variant="outlined"
            size="small"
            onClick={handleCancel}
            disabled={isSubmitting}
            data-testid={`rule-cancel-button-${row.id}`}
            sx={cancelButtonSx}
          >
            Cancel
          </Button>
        </Box>
      )}

      {canEdit && showInlineDelete && (
        <ClickAwayListener
          onClickAway={() => {
            if (showDeleteConfirm && !isSubmitting) {
              setShowDeleteConfirm(false);
            }
          }}
        >
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 1,
              flexShrink: 0,
            }}
          >
            <Button
              variant="contained"
              color="error"
              size="small"
              onClick={() => onDelete(row)}
              disabled={isSubmitting}
              data-testid={`rule-confirm-delete-button-${row.id}`}
              sx={{ minWidth: 64, height: 40, flexShrink: 0 }}
            >
              {isBusy ? (
                <CircularProgress size={20} color="inherit" />
              ) : (
                'Delete'
              )}
            </Button>
            <Button
              variant="outlined"
              size="small"
              onClick={handleCancel}
              disabled={isSubmitting}
              data-testid={`rule-cancel-button-${row.id}`}
              sx={cancelButtonSx}
            >
              Cancel
            </Button>
          </Box>
        </ClickAwayListener>
      )}

      {canEdit && !dirty && !showInlineDelete && (
        <IconButton
          aria-label={`delete rule ${row.id}`}
          size="small"
          onClick={() => {
            setShowDeleteConfirm(true);
          }}
          disabled={isSubmitting}
          data-testid={`rule-delete-button-${row.id}`}
          sx={{
            flexShrink: 0,
            color: 'text.secondary',
            '&:hover': {
              color: 'error.main',
            },
          }}
        >
          <DeleteOutlineIcon fontSize="small" />
        </IconButton>
      )}
    </Box>
  );
};
