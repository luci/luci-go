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

import {
  Add as AddIcon,
  ContentPaste as ContentPasteIcon,
  CopyAll as CopyAllIcon,
  Delete as DeleteIcon,
  ExpandMore as ExpandMoreIcon,
} from '@mui/icons-material';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Badge,
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  IconButton,
  Menu,
  MenuItem,
  Tooltip,
  Typography,
} from '@mui/material';
import { useMemo, useState } from 'react';

import {
  Column,
  COMMON_MESSAGES,
  OPERATOR_DISPLAY_NAMES,
} from '@/crystal_ball/constants';
import {
  useEditorUiState,
  useFiltersClipboard,
  useToast,
  UseEditorUiStateOptions,
} from '@/crystal_ball/hooks';
import { DataTestId } from '@/crystal_ball/tests';
import {
  MeasurementFilterColumn,
  MeasurementFilterColumn_ColumnDataType,
  measurementFilterColumn_ColumnDataTypeFromJSON,
  PerfFilter,
  PerfFilterDefault,
  PerfFilterDefault_FilterOperator,
  perfFilterDefault_FilterOperatorFromJSON,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { FilterEditorRow } from './filter_editor_row';

/**
 * Props for the FilterEditor component.
 */
interface FilterEditorProps {
  title?: string;
  filters: PerfFilter[];
  globalFilters?: readonly PerfFilter[];
  onUpdateFilters: (updatedFilters: PerfFilter[]) => void;
  dataSpecId: string;
  availableColumns: readonly MeasurementFilterColumn[];
  isLoadingColumns?: boolean;
  disableAccordion?: boolean;
  titleIcon?: React.ReactNode;
  uiStateOptions?: UseEditorUiStateOptions;
}

interface FilterEditorHeaderActionsProps {
  expanded: boolean;
  setExpanded: (val: boolean) => void;
  clipboardCount: number;
  setPasteMenuAnchor: (el: HTMLElement | null) => void;
  handleAddFilter: () => void;
  handleCopyFilters: () => void;
  handleClearFilters: () => void;
  disableAccordion?: boolean;
}

function FilterEditorHeaderActions({
  expanded,
  setExpanded,
  clipboardCount,
  setPasteMenuAnchor,
  handleAddFilter,
  handleCopyFilters,
  handleClearFilters,
  disableAccordion = false,
}: FilterEditorHeaderActionsProps) {
  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        ml: 1,
        alignSelf: 'center',
      }}
    >
      <Tooltip title={COMMON_MESSAGES.ADD_FILTER}>
        <Button
          data-testid={DataTestId.ADD_FILTER_BUTTON_TOP}
          startIcon={<AddIcon />}
          onClick={(e) => {
            e.stopPropagation();
            if (!disableAccordion && !expanded) {
              setExpanded(true);
            }
            handleAddFilter();
          }}
          variant="text"
          size="small"
          color="primary"
          sx={{
            textTransform: 'none',
            fontWeight: (theme) => theme.typography.fontWeightBold,
            ml: disableAccordion ? 1 : 0,
          }}
        >
          {COMMON_MESSAGES.ADD}
        </Button>
      </Tooltip>
      <Divider orientation="vertical" flexItem sx={{ mx: 0.5, my: 0.5 }} />
      <Box sx={{ display: 'flex', gap: 0.5 }}>
        <Tooltip title={COMMON_MESSAGES.PASTE_FILTERS}>
          <span>
            <IconButton
              aria-label={COMMON_MESSAGES.PASTE_FILTERS}
              size="small"
              sx={{
                color: 'text.secondary',
                '&.Mui-disabled': {
                  color: 'text.disabled',
                },
              }}
              onClick={(e) => {
                e.stopPropagation();
                setPasteMenuAnchor(e.currentTarget);
              }}
              disabled={clipboardCount === 0}
            >
              <Badge
                badgeContent={clipboardCount}
                color="primary"
                invisible={clipboardCount === 0}
                sx={{
                  '& .MuiBadge-badge': {
                    height: 16,
                    minWidth: 16,
                  },
                }}
              >
                <ContentPasteIcon fontSize="small" />
              </Badge>
            </IconButton>
          </span>
        </Tooltip>
        <Tooltip title={COMMON_MESSAGES.COPY_FILTERS}>
          <IconButton
            aria-label={COMMON_MESSAGES.COPY_FILTERS}
            size="small"
            sx={{ color: 'text.secondary' }}
            onClick={(e) => {
              e.stopPropagation();
              handleCopyFilters();
            }}
          >
            <CopyAllIcon fontSize="small" />
          </IconButton>
        </Tooltip>
        <Tooltip title={COMMON_MESSAGES.CLEAR_ALL_FILTERS}>
          <IconButton
            aria-label={COMMON_MESSAGES.CLEAR_ALL_FILTERS}
            size="small"
            onClick={(e) => {
              e.stopPropagation();
              handleClearFilters();
            }}
            sx={{ color: 'text.secondary' }}
          >
            <DeleteIcon fontSize="small" />
          </IconButton>
        </Tooltip>
      </Box>
    </Box>
  );
}

/**
 * Renders an editor for managing multiple performance filters.
 */
export function FilterEditor({
  title,
  filters,
  globalFilters,
  onUpdateFilters,
  dataSpecId,
  availableColumns,
  isLoadingColumns,
  disableAccordion = false,
  titleIcon,
  uiStateOptions,
}: FilterEditorProps) {
  const { showSuccessToast, showWarningToast } = useToast();
  const [expanded, setExpanded] = useEditorUiState({
    initialValue: false,
    ...uiStateOptions,
  });
  const [confirmClearOpen, setConfirmClearOpen] = useState(false);

  const [draggedIndex, setDraggedIndex] = useState<number | null>(null);

  const { clipboardCount, copyFilters, getClipboardFilters, clearClipboard } =
    useFiltersClipboard();
  const [pasteMenuAnchor, setPasteMenuAnchor] = useState<null | HTMLElement>(
    null,
  );

  const handleCopyFilters = () => {
    copyFilters(filters);
    showSuccessToast('Filters copied to clipboard');
  };

  const handlePasteFilters = (replace: boolean) => {
    const pastedFilters = getClipboardFilters();

    // Filter out filters that don't match available columns in the target context
    const validFilters = pastedFilters.filter((f) =>
      availableColumns.some((c) => c.column === f.column),
    );
    const invalidCount = pastedFilters.length - validFilters.length;

    const newFilters = validFilters.map((f) => ({
      ...f,
      id: `filter-${crypto.randomUUID()}`,
      dataSpecId: dataSpecId,
    }));
    if (replace) {
      onUpdateFilters(newFilters);
    } else {
      onUpdateFilters([...filters, ...newFilters]);
    }

    let msg = '';
    if (validFilters.length > 0) {
      msg += `${validFilters.length} filter(s) pasted.`;
    }
    if (invalidCount > 0) {
      msg += ` ${invalidCount} filter(s) ignored.`;
    }

    if (invalidCount > 0) {
      showWarningToast(msg.trim());
    } else if (validFilters.length > 0) {
      showSuccessToast(msg.trim());
    }

    setPasteMenuAnchor(null);
  };

  const handleClearFilters = () => {
    if (filters.length > 0) {
      setConfirmClearOpen(true);
    }
  };

  const handleDragStart = (index: number) => {
    setDraggedIndex(index);
  };

  const handleDrop = (targetIndex: number) => {
    if (draggedIndex === null || draggedIndex === targetIndex) return;
    const updatedFilters = [...filters];
    const [removed] = updatedFilters.splice(draggedIndex, 1);
    updatedFilters.splice(targetIndex, 0, removed);
    onUpdateFilters(updatedFilters);
    setDraggedIndex(null);
  };

  const handleAddFilter = () => {
    const newFilterId = `filter-${crypto.randomUUID()}`;
    const atpTestColumn = availableColumns.find(
      (c) => c.column === Column.ATP_TEST_NAME,
    );
    const selectedColumn = atpTestColumn ?? availableColumns[0];
    const isNumber =
      selectedColumn?.dataType ===
        MeasurementFilterColumn_ColumnDataType.INT64 ||
      selectedColumn?.dataType ===
        MeasurementFilterColumn_ColumnDataType.DOUBLE;

    const newFilter: PerfFilter = {
      id: newFilterId,
      column: selectedColumn?.column ?? '',
      dataSpecId: dataSpecId,
      displayName: COMMON_MESSAGES.NEW_FILTER,
      ...(isNumber
        ? {
            numberInput: {
              defaultValue: {
                values: [''],
                filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
              },
            },
          }
        : {
            textInput: {
              defaultValue: {
                values: [''],
                filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
              },
            },
          }),
    };
    onUpdateFilters([...filters, newFilter]);
  };

  const handleRemoveFilter = (index: number) => {
    const updatedFilters = [...filters];
    updatedFilters.splice(index, 1);
    onUpdateFilters(updatedFilters);
  };

  const handleFilterChange = (
    index: number,
    updatedFilterPart: Partial<PerfFilter>,
  ) => {
    const updatedFilters = [...filters];
    updatedFilters[index] = {
      ...updatedFilters[index],
      ...updatedFilterPart,
    };
    onUpdateFilters(updatedFilters);
  };

  const handleDefaultValueChange = <K extends keyof PerfFilterDefault>(
    index: number,
    key: K,
    value: PerfFilterDefault[K],
  ) => {
    const updatedFilters = [...filters];
    const currentFilter = updatedFilters[index];

    if (currentFilter.numberInput?.defaultValue) {
      updatedFilters[index] = {
        ...currentFilter,
        numberInput: {
          ...currentFilter.numberInput,
          defaultValue: {
            ...currentFilter.numberInput.defaultValue,
            [key]: value,
          },
        },
      };
      onUpdateFilters(updatedFilters);
    } else if (currentFilter.textInput?.defaultValue) {
      updatedFilters[index] = {
        ...currentFilter,
        textInput: {
          ...currentFilter.textInput,
          defaultValue: {
            ...currentFilter.textInput.defaultValue,
            [key]: value,
          },
        },
      };
      onUpdateFilters(updatedFilters);
    }
  };

  const primaryColumns = useMemo(
    () =>
      availableColumns
        .filter((c) => c.primary)
        .map((c) => c.column ?? '')
        .filter((c) => !!c)
        .sort((a, b) => a.localeCompare(b)),
    [availableColumns],
  );

  const secondaryColumns = useMemo(
    () =>
      availableColumns
        .filter((c) => !c.primary)
        .map((c) => c.column ?? '')
        .filter((c) => !!c)
        .sort((a, b) => a.localeCompare(b)),
    [availableColumns],
  );

  const renderFilterLabel = (filter: PerfFilter) => {
    const op =
      filter.textInput?.defaultValue?.filterOperator !== undefined
        ? perfFilterDefault_FilterOperatorFromJSON(
            filter.textInput.defaultValue.filterOperator,
          )
        : PerfFilterDefault_FilterOperator.EQUAL;
    const val = filter.textInput?.defaultValue?.values?.[0] ?? '';
    return `${filter.column} ${OPERATOR_DISPLAY_NAMES[op] ?? PerfFilterDefault_FilterOperator[op]} \"${val}\"`;
  };

  const content = (
    <>
      {isLoadingColumns ? (
        <Box sx={{ display: 'flex', justifySelf: 'center', p: 2 }}>
          <CircularProgress size={24} />
        </Box>
      ) : (
        <>
          {filters.length === 0 && (
            <Box
              sx={{
                border: '1px dashed',
                borderColor: 'divider',
                borderRadius: 1,
                p: 1.5,
                textAlign: 'center',
                mb: 1.5,
                bgcolor: 'background.paper',
              }}
            >
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ fontStyle: 'italic' }}
              >
                No filters applied.
              </Typography>
            </Box>
          )}
          {filters.map((filter, index) => {
            const colDef = availableColumns.find(
              (c) => c.column === filter.column,
            );
            const rawType = colDef
              ? colDef.dataType
              : MeasurementFilterColumn_ColumnDataType.COLUMN_DATA_TYPE_UNSPECIFIED;
            const dataType =
              measurementFilterColumn_ColumnDataTypeFromJSON(rawType);

            return (
              <FilterEditorRow
                key={filter.id}
                filter={filter}
                dataSpecId={dataSpecId}
                primaryColumns={primaryColumns}
                secondaryColumns={secondaryColumns}
                dataType={dataType}
                onUpdateColumn={(column) =>
                  handleFilterChange(index, { column })
                }
                onUpdateOperator={(operator) =>
                  handleDefaultValueChange(index, 'filterOperator', operator)
                }
                onUpdateValue={(values) =>
                  handleDefaultValueChange(index, 'values', values)
                }
                onRemove={() => handleRemoveFilter(index)}
                onDragStart={() => handleDragStart(index)}
                onDrop={() => handleDrop(index)}
                globalFilters={globalFilters}
                widgetFilters={filters}
              />
            );
          })}
        </>
      )}
    </>
  );

  const mainContent = disableAccordion ? (
    <Box
      sx={{
        mt: 1,
        display: 'block',
        alignItems: 'stretch',
        gap: 0,
      }}
    >
      {title && (
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 0.5,
            mb: 1,
            width: '100%',
          }}
        >
          {titleIcon}
          <Typography
            variant="caption"
            sx={{
              color: 'text.secondary',
              fontWeight: (theme) => theme.typography.fontWeightBold,
              textTransform: 'uppercase',
              lineHeight: 1,
            }}
          >
            {title}
          </Typography>
          <Box sx={{ flexGrow: 1 }} />
          <FilterEditorHeaderActions
            expanded={true}
            setExpanded={() => {}}
            clipboardCount={clipboardCount}
            setPasteMenuAnchor={setPasteMenuAnchor}
            handleAddFilter={handleAddFilter}
            handleCopyFilters={handleCopyFilters}
            handleClearFilters={handleClearFilters}
            disableAccordion={true}
          />
        </Box>
      )}
      {content}
    </Box>
  ) : (
    <Box sx={{ mt: 0 }}>
      <Accordion
        expanded={expanded}
        onChange={() => setExpanded(!expanded)}
        disableGutters
        elevation={0}
        square
        sx={{
          bgcolor: 'transparent',
          '&:before': { display: 'none' },
          border: 'none',
          boxShadow: 'none',
        }}
      >
        <AccordionSummary
          component="div"
          expandIcon={<ExpandMoreIcon />}
          aria-controls="filters-content"
          id="filters-header"
          sx={{
            px: 2,
            flexDirection: 'row-reverse',
            minHeight: (theme) => theme.spacing(4.5),
            '&.Mui-expanded': {
              minHeight: (theme) => theme.spacing(4.5),
            },
            '& .MuiAccordionSummary-expandIconWrapper': {
              marginRight: (theme) => theme.spacing(1),
            },
            '& .MuiAccordionSummary-content': {
              alignItems: 'center',
              flexWrap: 'nowrap',
              gap: 1,
              margin: '4px 0',
              width: '100%',
              '&.Mui-expanded': {
                margin: '4px 0',
              },
            },
          }}
        >
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 0.5,
              flexShrink: 0,
            }}
          >
            <Typography
              variant="caption"
              sx={{
                flexShrink: 0,
                color: 'text.secondary',
                fontWeight: (theme) => theme.typography.fontWeightBold,
                textTransform: 'uppercase',
                lineHeight: 1,
              }}
            >
              {title ?? COMMON_MESSAGES.FILTERS}
            </Typography>
          </Box>
          <Box
            sx={{
              display: 'flex',
              flexWrap: 'wrap',
              gap: 0.5,
              flexGrow: 1,
              mx: 1,
              minWidth: 0,
            }}
          >
            {!expanded &&
              filters.map((filter) => (
                <Chip
                  key={filter.id}
                  label={renderFilterLabel(filter)}
                  size="small"
                />
              ))}
            {!expanded && filters.length === 0 && (
              <Typography
                variant="caption"
                sx={{ color: 'text.secondary', fontStyle: 'italic' }}
              >
                {COMMON_MESSAGES.NO_FILTERS_APPLIED}
              </Typography>
            )}
          </Box>
          <FilterEditorHeaderActions
            expanded={expanded}
            setExpanded={setExpanded}
            clipboardCount={clipboardCount}
            setPasteMenuAnchor={setPasteMenuAnchor}
            handleAddFilter={handleAddFilter}
            handleCopyFilters={handleCopyFilters}
            handleClearFilters={handleClearFilters}
            disableAccordion={false}
          />
        </AccordionSummary>
        <AccordionDetails sx={{ pt: 1, pb: 2, px: 2 }}>
          {content}
        </AccordionDetails>
      </Accordion>
    </Box>
  );

  return (
    <>
      {mainContent}
      <Menu
        anchorEl={pasteMenuAnchor}
        open={Boolean(pasteMenuAnchor)}
        onClose={() => setPasteMenuAnchor(null)}
        onClick={(e) => e.stopPropagation()}
      >
        <MenuItem onClick={() => handlePasteFilters(false)}>
          {COMMON_MESSAGES.APPEND_FILTERS}
        </MenuItem>
        <MenuItem onClick={() => handlePasteFilters(true)}>
          {COMMON_MESSAGES.REPLACE_FILTERS}
        </MenuItem>
        <Divider />
        <MenuItem
          onClick={() => {
            clearClipboard();
            setPasteMenuAnchor(null);
            showSuccessToast('Clipboard cleared');
          }}
        >
          {COMMON_MESSAGES.CLEAR_CLIPBOARD}
        </MenuItem>
      </Menu>
      <Dialog
        open={confirmClearOpen}
        onClose={() => setConfirmClearOpen(false)}
      >
        <DialogTitle>Clear all filters</DialogTitle>
        <DialogContent>
          <Typography>Are you sure you want to clear all filters?</Typography>
        </DialogContent>
        <DialogActions sx={{ p: 2, pt: 0 }}>
          <Button onClick={() => setConfirmClearOpen(false)} color="inherit">
            Cancel
          </Button>
          <Button
            onClick={() => {
              onUpdateFilters([]);
              setConfirmClearOpen(false);
            }}
            color="error"
            variant="contained"
            disableElevation
            data-testid="confirm-dialog-confirm"
          >
            Clear
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
