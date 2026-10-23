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
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express governing permissions and
// limitations under the License.

import AddIcon from '@mui/icons-material/Add';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import PublicIcon from '@mui/icons-material/Public';
import SearchIcon from '@mui/icons-material/Search';
import {
  Alert,
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Stack,
  Typography,
} from '@mui/material';
import { useCallback, useMemo, useRef, useState } from 'react';

import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import {
  FilterCategory,
  FilterCategoryBuilder,
  useFilterState,
} from '@/fleet/components/filters/use_filters';
import {
  borders,
  controlHeights,
  elevations,
  fleetColors,
  iconSizes,
  radii,
} from '@/fleet/theme/tokens';

import { ALL_DEVICES_EXPRESSIONS, isAllDevicesFilter } from './utils';

export interface IrmFilterRowData {
  readonly id: string;
  readonly type: 'filter' | 'all';
  readonly expression: string;
}

const IrmAllDevicesRowItem = () => (
  <Box
    sx={{
      display: 'flex',
      alignItems: 'center',
      gap: 1,
      height: controlHeights.input,
      px: 1.5,
      backgroundColor: fleetColors.surface,
      border: borders.default,
      borderRadius: `${controlHeights.input / 2}px`,
      boxShadow: elevations.input,
      boxSizing: 'border-box',
      flex: 1,
      minWidth: 0,
      '&:hover': {
        borderColor: fleetColors.borderHover,
      },
    }}
  >
    <SearchIcon
      sx={{
        fontSize: iconSizes.inline,
        color: fleetColors.textTertiary,
        flexShrink: 0,
      }}
    />
    <Chip
      variant="filter"
      icon={<PublicIcon color="inherit" sx={{ fontSize: iconSizes.inline }} />}
      label="Matches all devices across the fleet"
      sx={{ cursor: 'default' }}
    />
  </Box>
);

interface IrmExpressionRowItemProps {
  readonly expression: string;
  readonly filterBuilders: Record<
    string,
    FilterCategoryBuilder<FilterCategory>
  >;
  readonly isBuildersLoading: boolean;
  readonly searchPlaceholder: string;
  readonly readOnly?: boolean;
  readonly onChange: (nextExpression: string) => void;
}

const IrmExpressionRowItem = ({
  expression,
  filterBuilders,
  isBuildersLoading,
  searchPlaceholder,
  readOnly = false,
  onChange,
}: IrmExpressionRowItemProps) => {
  const { filterValues } = useFilterState(
    filterBuilders,
    expression,
    onChange,
    {
      areFilterValuesLoading: isBuildersLoading,
    },
  );

  const filterCategoryDatas = useMemo(
    () => (filterValues ? Object.values(filterValues) : []),
    [filterValues],
  );

  return (
    <Box
      sx={{
        flex: 1,
        minWidth: 0,
        pointerEvents: readOnly ? 'none' : 'auto',
      }}
    >
      <FilterBar
        filterCategoryDatas={filterCategoryDatas}
        isLoading={isBuildersLoading}
        searchPlaceholder={searchPlaceholder}
        disableShortcut
      />
    </Box>
  );
};

interface IrmFilterRowItemProps {
  readonly row: IrmFilterRowData;
  readonly index: number;
  readonly filterBuilders: Record<
    string,
    FilterCategoryBuilder<FilterCategory>
  >;
  readonly isBuildersLoading: boolean;
  readonly searchPlaceholder: string;
  readonly readOnly?: boolean;
  readonly onChange: (id: string, nextExpression: string) => void;
  readonly onDelete: (id: string) => void;
}

const IrmFilterRowItem = ({
  row,
  index,
  filterBuilders,
  isBuildersLoading,
  searchPlaceholder,
  readOnly = false,
  onChange,
  onDelete,
}: IrmFilterRowItemProps) => (
  <Box
    data-testid="irm-filter-row"
    sx={{
      display: 'flex',
      alignItems: 'center',
      gap: 1.5,
      width: '100%',
    }}
  >
    {row.type === 'all' ? (
      <IrmAllDevicesRowItem />
    ) : (
      <IrmExpressionRowItem
        expression={row.expression}
        filterBuilders={filterBuilders}
        isBuildersLoading={isBuildersLoading}
        searchPlaceholder={searchPlaceholder}
        readOnly={readOnly}
        onChange={(next) => onChange(row.id, next)}
      />
    )}

    {!readOnly && (
      <IconButton
        aria-label={
          row.type === 'all'
            ? 'delete match all devices'
            : `delete filter ${index + 1}`
        }
        size="small"
        color="error"
        onClick={() => onDelete(row.id)}
        data-testid="irm-filter-delete-btn"
        sx={{ flexShrink: 0 }}
      >
        <DeleteOutlineIcon fontSize="small" />
      </IconButton>
    )}
  </Box>
);

export interface IrmFilterDialogProps {
  readonly initialExpressions: readonly string[];
  readonly filterBuilders: Record<
    string,
    FilterCategoryBuilder<FilterCategory>
  >;
  readonly isBuildersLoading: boolean;
  readonly searchPlaceholder: string;
  readonly readOnly?: boolean;
  readonly onCancel: () => void;
  readonly onSave: (savedExpressions: readonly string[]) => void;
}

const buildInitialRows = (
  expressions: readonly string[],
): readonly IrmFilterRowData[] => {
  if (isAllDevicesFilter(expressions)) {
    return [{ id: '0', type: 'all', expression: '' }];
  }
  return expressions.flatMap((e, i) => {
    const expr = e.trim();
    return expr ? [{ id: String(i), type: 'filter', expression: expr }] : [];
  });
};

export const IrmFilterDialog = ({
  initialExpressions,
  filterBuilders,
  isBuildersLoading,
  searchPlaceholder,
  readOnly = false,
  onCancel,
  onSave,
}: IrmFilterDialogProps) => {
  const [rows, setRows] = useState(() => buildInitialRows(initialExpressions));
  const nextIdRef = useRef(rows.length);
  const nextId = () => String(nextIdRef.current++);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const hasAllDevicesRow = rows.some((r) => r.type === 'all');

  const handleRowChange = useCallback((id: string, nextExpression: string) => {
    setRows((prev) =>
      prev.map((r) => (r.id === id ? { ...r, expression: nextExpression } : r)),
    );
    setErrorMessage(null);
  }, []);

  const handleDeleteRow = useCallback((id: string) => {
    setRows((prev) => prev.filter((r) => r.id !== id));
    setErrorMessage(null);
  }, []);

  const handleAddRow = useCallback(() => {
    setRows((prev) => [
      ...prev,
      { id: nextId(), type: 'filter', expression: '' },
    ]);
    setErrorMessage(null);
  }, []);

  const handleSetMatchAll = useCallback(() => {
    setRows([{ id: nextId(), type: 'all', expression: '' }]);
    setErrorMessage(null);
  }, []);

  const handleSave = useCallback(() => {
    if (rows.length === 0) {
      onSave([]);
      return;
    }

    if (hasAllDevicesRow) {
      onSave(ALL_DEVICES_EXPRESSIONS);
      return;
    }

    const hasEmpty = rows.some((r) => r.expression.trim() === '');
    if (hasEmpty) {
      setErrorMessage('Filter expression cannot be empty');
      return;
    }

    onSave(rows.map((r) => r.expression.trim()));
  }, [hasAllDevicesRow, onSave, rows]);

  return (
    <Dialog
      open
      onClose={onCancel}
      maxWidth="md"
      fullWidth
      data-testid="irm-filter-dialog"
    >
      <DialogTitle sx={{ fontWeight: 'bold', pb: 1 }}>
        {readOnly ? 'View Incident Filters' : 'Edit Incident Filters'}
      </DialogTitle>
      <DialogContent
        sx={{
          display: 'flex',
          flexDirection: 'column',
          gap: 2,
          pt: 2,
          minHeight: 180,
        }}
      >
        {errorMessage && (
          <Alert
            severity="error"
            onClose={() => setErrorMessage(null)}
            data-testid="irm-filter-error"
          >
            {errorMessage}
          </Alert>
        )}

        {rows.length === 0 ? (
          <Box
            sx={{
              p: 2,
              border: `1px dashed ${fleetColors.border}`,
              borderRadius: radii.sm,
              textAlign: 'center',
            }}
            data-testid="irm-filter-empty-state"
          >
            <Typography variant="body2" color="text.secondary">
              {readOnly
                ? 'No filters configured for this incident.'
                : 'No filters configured. Click "Add filter" to target specific devices, or "Match all devices" for the entire fleet.'}
            </Typography>
          </Box>
        ) : (
          <Stack spacing={1.5}>
            {rows.map((row, index) => (
              <IrmFilterRowItem
                key={row.id}
                row={row}
                index={index}
                filterBuilders={filterBuilders}
                isBuildersLoading={isBuildersLoading}
                searchPlaceholder={searchPlaceholder}
                readOnly={readOnly}
                onChange={handleRowChange}
                onDelete={handleDeleteRow}
              />
            ))}
          </Stack>
        )}

        {!readOnly && (
          <Stack direction="row" spacing={1} sx={{ alignSelf: 'flex-start' }}>
            <Button
              variant="outlined"
              size="small"
              startIcon={<AddIcon />}
              onClick={handleAddRow}
              disabled={hasAllDevicesRow}
              data-testid="irm-filter-add-row"
              sx={{
                textTransform: 'none',
                fontWeight: 500,
              }}
            >
              Add filter
            </Button>
            <Button
              variant="outlined"
              size="small"
              startIcon={<PublicIcon />}
              onClick={handleSetMatchAll}
              disabled={hasAllDevicesRow}
              data-testid="irm-filter-all-btn"
              sx={{
                textTransform: 'none',
                fontWeight: 500,
              }}
            >
              Match all devices
            </Button>
          </Stack>
        )}
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2.5 }}>
        {readOnly ? (
          <Button
            disableElevation
            onClick={onCancel}
            data-testid="irm-filter-close"
          >
            Close
          </Button>
        ) : (
          <>
            <Button
              disableElevation
              onClick={onCancel}
              data-testid="irm-filter-cancel"
              sx={{ ml: 'auto' }}
            >
              Cancel
            </Button>
            <Button
              disableElevation
              variant="contained"
              onClick={handleSave}
              data-testid="irm-filter-save"
            >
              Save
            </Button>
          </>
        )}
      </DialogActions>
    </Dialog>
  );
};
