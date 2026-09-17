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

import AddIcon from '@mui/icons-material/Add';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  IconButton,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';

import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import {
  FilterCategory,
  FilterCategoryBuilder,
  useFilterState,
} from '@/fleet/components/filters/use_filters';
import { colors } from '@/fleet/theme/colors';
import { PriorityRule } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { usePriorityRules } from './use_priority_rules';
import { useRepairQueueFilterBuilders } from './use_repair_queue_filter_builders';

const MAX_PRIORITY_RULES = 5;
const DEFAULT_VISIBLE_RULES = 3;
const MIN_RULE_WEIGHT = -1000000;
const MAX_RULE_WEIGHT = 1000000;

interface EditableRuleRow {
  readonly id: string;
  readonly isDraft: boolean;
  readonly expressionAip160: string;
  readonly weight: string;
  readonly backendExpressionAip160: string;
  readonly backendWeight: string;
}

const ruleToEditableRow = (rule: PriorityRule): EditableRuleRow => ({
  id: rule.id,
  isDraft: false,
  expressionAip160: rule.expressionAip160,
  weight: rule.weight,
  backendExpressionAip160: rule.expressionAip160,
  backendWeight: rule.weight,
});

const isRowDirty = (row: EditableRuleRow): boolean => {
  if (row.isDraft) return true;
  return (
    row.expressionAip160 !== row.backendExpressionAip160 ||
    row.weight !== row.backendWeight
  );
};

interface PriorityRuleRowProps {
  readonly row: EditableRuleRow;
  readonly index: number;
  readonly filterBuilders: Record<
    string,
    FilterCategoryBuilder<FilterCategory>
  >;
  readonly isBuildersLoading: boolean;
  readonly isBusy: boolean;
  readonly isSubmitting: boolean;
  readonly onFilterChange: (id: string, nextAip160: string) => void;
  readonly onWeightChange: (id: string, weight: string) => void;
  readonly onApply: (row: EditableRuleRow) => void;
  readonly onDelete: (row: EditableRuleRow) => void;
}
const formatPriorityRuleError = (
  expressionAip160: string,
  filterErrors: readonly string[] | undefined,
): string | null => {
  if (filterErrors && filterErrors.length > 0) {
    const formattedErrors = filterErrors.map((err) => {
      if (/\bid\b is not a valid filter/i.test(err)) {
        return '"id" is not a valid filter field for repair tasks (use "dut_id" for device hostnames)';
      }
      if (err.includes('OR between filters is not supported yet')) {
        return 'OR operations between different filter fields are not supported';
      }
      return err;
    });

    const trimmedExpr = expressionAip160.trim();
    if (trimmedExpr) {
      return `Invalid rule expression "${trimmedExpr}": ${formattedErrors.join('; ')}`;
    }
    return formattedErrors.join('; ');
  }

  return null;
};

const PriorityRuleRow = ({
  row,
  index,
  filterBuilders,
  isBuildersLoading,
  isBusy,
  isSubmitting,
  onFilterChange,
  onWeightChange,
  onApply,
  onDelete,
}: PriorityRuleRowProps) => {
  const handleFilterChange = useCallback(
    (nextAip160: string) => {
      onFilterChange(row.id, nextAip160);
    },
    [onFilterChange, row.id],
  );

  const { filterValues, filterErrors } = useFilterState(
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

  const ruleError = useMemo(() => {
    if (isBuildersLoading) {
      return null;
    }
    return formatPriorityRuleError(row.expressionAip160, filterErrors);
  }, [isBuildersLoading, row.expressionAip160, filterErrors]);

  const dirty = isRowDirty(row);

  // A draft counts as dirty from the moment it is created, so a plain
  // `dirty` swap would leave a brand new row with no way to back out of it.
  // Until the draft has a filter there is nothing to apply anyway, so it
  // keeps the delete button and Apply takes over once the row means
  // something. Clearing the last chip brings delete back.
  const isUntouchedDraft = row.isDraft && row.expressionAip160.trim() === '';
  const showApply = dirty && !isUntouchedDraft;

  return (
    <Box
      data-testid={`priority-rule-row-${row.id}`}
      sx={{
        display: 'flex',
        flexDirection: 'column',
        gap: 0.75,
        width: '100%',
      }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 1.5,
          width: '100%',
        }}
      >
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <FilterBar
            filterCategoryDatas={filterCategoryDatas}
            isLoading={isBuildersLoading}
            searchPlaceholder="Add rule filter (e.g. pool, board, model, dut_state)..."
            disableShortcut
          />
        </Box>

        <TextField
          label="Pts"
          type="number"
          value={row.weight}
          onChange={(e) => onWeightChange(row.id, e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !isSubmitting && !ruleError) {
              e.preventDefault();
              onApply(row);
            }
          }}
          size="small"
          disabled={isBusy}
          sx={{ width: 100, flexShrink: 0 }}
          inputProps={{
            min: MIN_RULE_WEIGHT,
            max: MAX_RULE_WEIGHT,
            'data-testid': `rule-weight-input-${row.id}`,
            'aria-label': `Rule ${index + 1} points weight`,
          }}
        />

        {/*
          One fixed-width slot holding a single control. Apply takes over the
          delete slot while the row is dirty rather than appearing beside it,
          because inserting a button mid-row shifted the Pts field sideways on
          every keystroke. The width is pinned so swapping a text button for an
          icon button does not move anything either.
        */}
        <Box
          sx={{
            width: 72,
            flexShrink: 0,
            display: 'flex',
            justifyContent: 'center',
          }}
        >
          {showApply ? (
            <Button
              variant="contained"
              color="primary"
              size="small"
              onClick={() => onApply(row)}
              disabled={isSubmitting || !!ruleError}
              data-testid={`rule-apply-button-${row.id}`}
              sx={{ minWidth: 64, height: 40 }}
            >
              {isBusy ? (
                <CircularProgress size={20} color="inherit" />
              ) : (
                'Apply'
              )}
            </Button>
          ) : (
            <IconButton
              aria-label={`delete rule ${row.id}`}
              size="small"
              onClick={() => onDelete(row)}
              disabled={isSubmitting}
              data-testid={`rule-delete-button-${row.id}`}
              sx={{ color: '#d32f2f' }}
            >
              <DeleteOutlineIcon fontSize="small" />
            </IconButton>
          )}
        </Box>
      </Box>

      {ruleError && (
        <Alert
          severity="error"
          sx={{
            py: 0.25,
            px: 1.5,
            fontSize: '0.8125rem',
            '& .MuiAlert-icon': { py: 0.5 },
          }}
          data-testid={`priority-rule-error-${row.id}`}
        >
          {ruleError}
        </Alert>
      )}
    </Box>
  );
};

export const PriorityRulesPanel = () => {
  const {
    rules,
    isLoading: isRulesLoading,
    isError,
    error,
    createRule,
    isCreating,
    updateRule,
    isUpdating,
    deleteRule,
    isDeleting,
  } = usePriorityRules();

  const { filterBuilders, isLoading: isBuildersLoading } =
    useRepairQueueFilterBuilders();

  const [rows, setRows] = useState<readonly EditableRuleRow[]>([]);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isExpanded, setIsExpanded] = useState<boolean>(false);
  const [actionInProgressId, setActionInProgressId] = useState<string | null>(
    null,
  );

  // Sync rows with remote rules
  useEffect(() => {
    setRows((prevRows) => {
      if (prevRows.length === 0) {
        return rules.map(ruleToEditableRow);
      }

      const updatedRemoteRows: EditableRuleRow[] = rules.map((rule) => {
        const existing = prevRows.find((r) => r.id === rule.id);
        if (!existing) {
          return ruleToEditableRow(rule);
        }
        if (isRowDirty(existing)) {
          return {
            ...existing,
            backendExpressionAip160: rule.expressionAip160,
            backendWeight: rule.weight,
          };
        }
        return ruleToEditableRow(rule);
      });

      const activeDraftRows = prevRows.filter((r) => r.isDraft);
      return [...updatedRemoteRows, ...activeDraftRows];
    });
  }, [rules]);

  const handleFieldChange = useCallback(
    (id: string, field: 'expressionAip160' | 'weight', value: string) => {
      setRows((prev) =>
        prev.map((row) => (row.id === id ? { ...row, [field]: value } : row)),
      );
      setErrorMessage(null);
    },
    [],
  );

  const handleAddRule = () => {
    if (rows.length >= MAX_PRIORITY_RULES) return;
    const newDraftId = `draft-${Date.now()}`;
    const newDraftRow: EditableRuleRow = {
      id: newDraftId,
      isDraft: true,
      expressionAip160: '',
      weight: '0',
      backendExpressionAip160: '',
      backendWeight: '0',
    };
    setRows((prev) => [...prev, newDraftRow]);
    setIsExpanded(true);
    setErrorMessage(null);
  };

  const handleDeleteRow = async (row: EditableRuleRow) => {
    setErrorMessage(null);
    if (row.isDraft) {
      setRows((prev) => prev.filter((r) => r.id !== row.id));
      return;
    }

    try {
      setActionInProgressId(row.id);
      await deleteRule({ id: row.id });
    } catch (err: unknown) {
      const msg =
        err instanceof Error ? err.message : 'Failed to delete priority rule';
      setErrorMessage(msg);
    } finally {
      setActionInProgressId(null);
    }
  };

  const handleApplyRow = async (row: EditableRuleRow) => {
    setErrorMessage(null);

    const trimmedExpr = row.expressionAip160.trim();
    if (!trimmedExpr) {
      setErrorMessage('Filter expression cannot be empty');
      return;
    }

    const trimmedWeight = row.weight.trim();
    if (!/^-?\d+$/.test(trimmedWeight)) {
      setErrorMessage(
        'Weight must be a valid integer between -1,000,000 and 1,000,000',
      );
      return;
    }

    const parsedWeight = parseInt(trimmedWeight, 10);
    if (
      isNaN(parsedWeight) ||
      parsedWeight < MIN_RULE_WEIGHT ||
      parsedWeight > MAX_RULE_WEIGHT
    ) {
      setErrorMessage(
        'Weight must be a valid integer between -1,000,000 and 1,000,000',
      );
      return;
    }

    try {
      setActionInProgressId(row.id);
      if (row.isDraft) {
        await createRule({
          priorityRule: {
            id: '0',
            expressionAip160: trimmedExpr,
            weight: parsedWeight.toString(),
          },
        });
        setRows((prev) => prev.filter((r) => r.id !== row.id));
      } else {
        await updateRule({
          id: row.id,
          expressionAip160: trimmedExpr,
          weight: parsedWeight.toString(),
        });
        setRows((prev) =>
          prev.map((r) =>
            r.id === row.id
              ? {
                  ...r,
                  expressionAip160: trimmedExpr,
                  weight: parsedWeight.toString(),
                  backendExpressionAip160: trimmedExpr,
                  backendWeight: parsedWeight.toString(),
                }
              : r,
          ),
        );
      }
    } catch (err: unknown) {
      const msg =
        err instanceof Error ? err.message : 'Failed to apply priority rule';
      setErrorMessage(msg);
    } finally {
      setActionInProgressId(null);
    }
  };

  const isSubmitting = isCreating || isUpdating || isDeleting;
  const visibleRows = isExpanded ? rows : rows.slice(0, DEFAULT_VISIBLE_RULES);
  const hiddenCount = Math.max(0, rows.length - DEFAULT_VISIBLE_RULES);

  return (
    <Box sx={{ width: '100%' }}>
      <Typography
        variant="h6"
        sx={{ mt: 2, mb: 2, fontSize: '16px', fontWeight: 'bold' }}
      >
        Priority Scoring Rules
      </Typography>

      {errorMessage && (
        <Alert
          severity="error"
          onClose={() => setErrorMessage(null)}
          sx={{ mb: 2 }}
        >
          {errorMessage}
        </Alert>
      )}

      {isError && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error?.message || 'Failed to load priority scoring rules.'}
        </Alert>
      )}

      {isRulesLoading ? (
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            py: 4,
          }}
        >
          <CircularProgress size={32} />
        </Box>
      ) : (
        <Stack spacing={1.5}>
          {rows.length === 0 ? (
            <Box
              sx={{
                p: 2,
                textAlign: 'center',
                backgroundColor: colors.grey[50],
                borderRadius: 1,
                border: `1px dashed ${colors.grey[300]}`,
              }}
            >
              <Typography variant="body2" color="text.secondary">
                No priority scoring rules configured. Click &ldquo;+ Add
                rule&rdquo; below to create your first rule.
              </Typography>
            </Box>
          ) : (
            visibleRows.map((row, index) => {
              const isRowBusy = isSubmitting && actionInProgressId === row.id;

              return (
                <PriorityRuleRow
                  key={row.id}
                  row={row}
                  index={index}
                  filterBuilders={filterBuilders}
                  isBuildersLoading={isBuildersLoading}
                  isBusy={isRowBusy}
                  isSubmitting={isSubmitting}
                  onFilterChange={(id, nextAip160) =>
                    handleFieldChange(id, 'expressionAip160', nextAip160)
                  }
                  onWeightChange={(id, weight) =>
                    handleFieldChange(id, 'weight', weight)
                  }
                  onApply={handleApplyRow}
                  onDelete={handleDeleteRow}
                />
              );
            })
          )}

          <Box
            sx={{
              mt: 1,
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'flex-start',
              gap: 1,
            }}
          >
            <Button
              variant="outlined"
              size="small"
              startIcon={<AddIcon />}
              onClick={handleAddRule}
              disabled={rows.length >= MAX_PRIORITY_RULES || isSubmitting}
              data-testid="add-priority-rule-button"
              sx={{
                textTransform: 'none',
                fontWeight: 500,
              }}
            >
              {rows.length >= MAX_PRIORITY_RULES
                ? `Limit of ${MAX_PRIORITY_RULES} rules reached`
                : 'Add rule'}
            </Button>

            {hiddenCount > 0 && !isExpanded && (
              <Button
                variant="text"
                size="small"
                onClick={() => setIsExpanded(true)}
                startIcon={<ExpandMoreIcon />}
                data-testid="show-more-rules-button"
                sx={{
                  textTransform: 'none',
                  p: 0,
                  minWidth: 'auto',
                }}
              >
                Show {hiddenCount} more {hiddenCount === 1 ? 'rule' : 'rules'}
              </Button>
            )}

            {isExpanded && rows.length > DEFAULT_VISIBLE_RULES && (
              <Button
                variant="text"
                size="small"
                onClick={() => setIsExpanded(false)}
                startIcon={<ExpandLessIcon />}
                data-testid="show-less-rules-button"
                sx={{
                  textTransform: 'none',
                  p: 0,
                  minWidth: 'auto',
                }}
              >
                Show less rules
              </Button>
            )}
          </Box>
        </Stack>
      )}
    </Box>
  );
};
