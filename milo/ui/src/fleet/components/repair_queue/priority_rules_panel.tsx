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
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Stack,
  Typography,
} from '@mui/material';
import { useCallback, useState } from 'react';

import {
  FilterCategory,
  FilterCategoryBuilder,
} from '@/fleet/components/filters/use_filters';
import { colors } from '@/fleet/theme/colors';

import {
  EditableRuleRow,
  MAX_RULE_WEIGHT,
  MIN_RULE_WEIGHT,
  RepairPriorityRule,
} from './editable_rule_row';
import { PriorityRuleRow } from './priority_rule_row';
import { useEditableRuleRows } from './use_editable_rule_rows';

const MAX_PRIORITY_RULES = 5;
const DEFAULT_VISIBLE_RULES = 3;

export interface PriorityRulesPanelProps {
  readonly rules: readonly RepairPriorityRule[];
  readonly isLoading: boolean;
  readonly isError: boolean;
  readonly error: unknown;
  readonly createRule: (req: {
    readonly priorityRule: RepairPriorityRule;
  }) => Promise<unknown>;
  readonly isCreating: boolean;
  readonly updateRule: (req: {
    readonly id: string;
    readonly expressionAip160: string;
    readonly weight: string;
  }) => Promise<unknown>;
  readonly isUpdating: boolean;
  readonly deleteRule: (req: { readonly id: string }) => Promise<unknown>;
  readonly isDeleting: boolean;
  readonly filterBuilders: Record<
    string,
    FilterCategoryBuilder<FilterCategory>
  >;
  readonly isBuildersLoading: boolean;
  readonly canEdit: boolean;
  /** Placeholder shown in the rule filter input while editing. */
  readonly searchPlaceholder?: string;
}

export const PriorityRulesPanel = ({
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
  filterBuilders,
  isBuildersLoading,
  canEdit,
  searchPlaceholder,
}: PriorityRulesPanelProps) => {
  const [rows, setRows] = useEditableRuleRows(rules);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isExpanded, setIsExpanded] = useState<boolean>(false);
  const [actionInProgressId, setActionInProgressId] = useState<string | null>(
    null,
  );

  const handleFieldChange = useCallback(
    (id: string, field: 'expressionAip160' | 'weight', value: string) => {
      if (!canEdit) return;
      setRows((prev) =>
        prev.map((row) => (row.id === id ? { ...row, [field]: value } : row)),
      );
      setErrorMessage(null);
    },
    [canEdit, setRows],
  );

  const handleAddRule = () => {
    if (!canEdit || rows.length >= MAX_PRIORITY_RULES) return;
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
    if (!canEdit) return;
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
    if (!canEdit) return;
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
          {(error instanceof Error && error.message) ||
            'Failed to load priority scoring rules.'}
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
                {canEdit ? (
                  <>
                    No priority scoring rules configured. Click &ldquo;+ Add
                    rule&rdquo; below to create your first rule.
                  </>
                ) : (
                  'No priority scoring rules configured.'
                )}
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
                  canEdit={canEdit}
                  searchPlaceholder={searchPlaceholder}
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
            {canEdit && (
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
            )}

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
