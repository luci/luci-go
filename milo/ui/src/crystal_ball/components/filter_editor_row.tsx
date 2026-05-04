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
  ContentCopy as ContentCopyIcon,
  Delete as DeleteIcon,
  DragIndicator as DragIndicatorIcon,
} from '@mui/icons-material';
import {
  Autocomplete,
  Box,
  Checkbox,
  Divider,
  IconButton,
  MenuItem,
  Select,
  SelectChangeEvent,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
} from '@mui/material';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useParams } from 'react-router';
import { useDebounce } from 'react-use';

import {
  AUTOCOMPLETE_DEBOUNCE_DELAY_MS,
  COMMON_MESSAGES,
  MAX_SUGGEST_RESULTS,
  OPERATOR_DISPLAY_NAMES,
  TYPE_TO_OPERATORS,
} from '@/crystal_ball/constants';
import {
  useFiltersClipboard,
  useSuggestMeasurementFilterValues,
} from '@/crystal_ball/hooks';
import {
  COMPACT_FILTER_ROW_SX,
  COMPACT_SELECT_SX,
  COMPACT_TEXTFIELD_SX,
} from '@/crystal_ball/styles';
import { DataTestId } from '@/crystal_ball/tests';
import { buildFilterString } from '@/crystal_ball/utils';
import {
  MeasurementFilterColumn_ColumnDataType,
  PerfFilter,
  PerfFilterDefault_FilterOperator,
  perfFilterDefault_FilterOperatorFromJSON,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

interface FilterEditorRowProps {
  filter: PerfFilter;
  dataSpecId: string;
  primaryColumns: string[];
  secondaryColumns: string[];
  dataType: MeasurementFilterColumn_ColumnDataType;
  isCheckboxFilter?: boolean;
  onUpdateColumn: (column: string) => void;
  onUpdateOperator: (operator: PerfFilterDefault_FilterOperator) => void;
  onUpdateValue: (values: string[]) => void;
  onRemove: () => void;
  onDragStart: () => void;
  onDrop: () => void;
  globalFilters?: readonly PerfFilter[];
  widgetFilters?: readonly PerfFilter[];
}

/**
 * Renders a single row in the filter editor, allowing the user to select a column,
 * operator, and value. Supports custom rendering for checkbox filters.
 */
export function FilterEditorRow({
  filter,
  dataSpecId,
  primaryColumns,
  secondaryColumns,
  dataType,
  isCheckboxFilter = false,
  onUpdateColumn,
  onUpdateOperator,
  onUpdateValue,
  onRemove,
  onDragStart,
  onDrop,
  globalFilters,
  widgetFilters,
}: FilterEditorRowProps) {
  const { copyFilters } = useFiltersClipboard();
  const rowRef = useRef<HTMLDivElement>(null);
  const activeInput = filter.numberInput ?? filter.textInput;
  const initialValue = activeInput?.defaultValue?.values?.[0] ?? '';

  const currentOperator =
    activeInput?.defaultValue?.filterOperator !== undefined
      ? perfFilterDefault_FilterOperatorFromJSON(
          activeInput.defaultValue.filterOperator,
        )
      : PerfFilterDefault_FilterOperator.EQUAL;

  const isMultiSelect =
    currentOperator === PerfFilterDefault_FilterOperator.IN ||
    currentOperator === PerfFilterDefault_FilterOperator.NOT_IN;

  const [inputValue, setInputValue] = useState(
    isMultiSelect ? '' : initialValue,
  );
  const [debouncedQuery, setDebouncedQuery] = useState(inputValue);
  const [isFocused, setIsFocused] = useState(false);

  const prevOperatorRef = useRef(currentOperator);
  useEffect(() => {
    if (currentOperator !== prevOperatorRef.current) {
      prevOperatorRef.current = currentOperator;
      if (isMultiSelect) {
        setInputValue('');
      } else {
        setInputValue(initialValue);
      }
    }
  }, [currentOperator, initialValue, isMultiSelect]);

  useDebounce(
    () => {
      setDebouncedQuery(inputValue);
    },
    AUTOCOMPLETE_DEBOUNCE_DELAY_MS,
    [inputValue],
  );

  const { dashboardId } = useParams<{ dashboardId: string }>();
  const parent = dashboardId
    ? `dashboardStates/${dashboardId}/dataSpecs/${dataSpecId}`
    : '';

  const filterString = useMemo(() => {
    return buildFilterString(
      [...(globalFilters ?? []), ...(widgetFilters ?? [])],
      filter.id,
    );
  }, [globalFilters, widgetFilters, filter.id]);

  const { data: suggestionData, isLoading } = useSuggestMeasurementFilterValues(
    {
      parent,
      column: filter.column,
      query: isCheckboxFilter ? '' : debouncedQuery,
      maxResultCount: MAX_SUGGEST_RESULTS,
      filter: isCheckboxFilter ? '' : filterString,
    },
    {
      enabled:
        !!parent &&
        !!filter.column &&
        (isCheckboxFilter || (debouncedQuery.length > 0 && isFocused)),
      retry: false,
    },
  );

  const options = useMemo(
    () =>
      Array.from(
        new Set(
          (suggestionData?.values ?? []).filter(
            (v): v is string => typeof v === 'string',
          ),
        ),
      ),
    [suggestionData?.values],
  );

  const sortedOptions = useMemo(() => {
    return [...options].sort((a, b) => a.localeCompare(b));
  }, [options]);

  const handleBlur = () => {
    if (!isMultiSelect && inputValue !== initialValue) {
      onUpdateValue([inputValue]);
    }
  };

  return (
    <Box
      ref={rowRef}
      sx={{
        ...COMPACT_FILTER_ROW_SX,
        gridTemplateColumns: isCheckboxFilter
          ? 'auto 2fr 5fr auto'
          : 'auto 2fr 1fr 4fr auto',
        mb: 1,
        gap: 1,
      }}
      onDragOver={(e) => e.preventDefault()}
      onDrop={onDrop}
      data-testid={DataTestId.FILTER_EDITOR_ROW}
    >
      <Box
        draggable
        onDragStart={(e) => {
          if (rowRef.current && e.dataTransfer) {
            const rect = rowRef.current.getBoundingClientRect();
            const x = e.clientX - rect.left;
            const y = e.clientY - rect.top;
            e.dataTransfer.setDragImage(rowRef.current, x, y);
          }
          onDragStart();
        }}
        sx={{ display: 'inline-flex', cursor: 'grab' }}
        data-testid={DataTestId.FILTER_DRAG_HANDLE}
      >
        <DragIndicatorIcon sx={{ color: 'text.secondary', mr: 0.5 }} />
      </Box>
      <Select
        value={filter.column}
        onChange={(e: SelectChangeEvent<string>) =>
          onUpdateColumn(e.target.value)
        }
        size="small"
        displayEmpty
        inputProps={{ 'aria-label': COMMON_MESSAGES.COLUMN }}
        sx={COMPACT_SELECT_SX}
        MenuProps={{ PaperProps: { style: { maxHeight: 400 } } }}
      >
        {filter.column &&
          !primaryColumns.includes(filter.column) &&
          !secondaryColumns.includes(filter.column) && (
            <MenuItem key={filter.column} value={filter.column}>
              {filter.column}
            </MenuItem>
          )}
        {primaryColumns.map((col) => (
          <MenuItem key={col} value={col}>
            {col}
          </MenuItem>
        ))}

        {secondaryColumns.length > 0 && [
          <Divider key="divider" />,
          ...secondaryColumns.map((col) => (
            <MenuItem key={col} value={col}>
              {col}
            </MenuItem>
          )),
        ]}
      </Select>

      {dataType === MeasurementFilterColumn_ColumnDataType.BOOLEAN ? (
        <Box
          sx={{ gridColumn: 'span 2', display: 'flex', alignItems: 'center' }}
        >
          <ToggleButtonGroup
            size="small"
            value={
              currentOperator === PerfFilterDefault_FilterOperator.IS_TRUE
                ? 'true'
                : currentOperator === PerfFilterDefault_FilterOperator.IS_FALSE
                  ? 'false'
                  : null
            }
            exclusive
            onChange={(_event, newValue) => {
              if (newValue === null) return; // Enforce that one must remain selected
              if (newValue === 'true') {
                onUpdateOperator(PerfFilterDefault_FilterOperator.IS_TRUE);
              } else if (newValue === 'false') {
                onUpdateOperator(PerfFilterDefault_FilterOperator.IS_FALSE);
              }
            }}
            sx={{ height: '32px' }}
          >
            <ToggleButton value="true" sx={{ px: 2 }}>
              True
            </ToggleButton>
            <ToggleButton value="false" sx={{ px: 2 }}>
              False
            </ToggleButton>
          </ToggleButtonGroup>
        </Box>
      ) : (
        <>
          {!isCheckboxFilter && (
            <Select
              value={
                activeInput?.defaultValue?.filterOperator !== undefined
                  ? perfFilterDefault_FilterOperatorFromJSON(
                      activeInput.defaultValue.filterOperator,
                    )
                  : PerfFilterDefault_FilterOperator.EQUAL
              }
              onChange={(e: SelectChangeEvent<number>) => {
                onUpdateOperator(Number(e.target.value));
              }}
              size="small"
              displayEmpty
              inputProps={{ 'aria-label': COMMON_MESSAGES.OPERATOR }}
              sx={COMPACT_SELECT_SX}
              MenuProps={{ PaperProps: { style: { maxHeight: 400 } } }}
            >
              {(TYPE_TO_OPERATORS[dataType] ?? []).map((opEnum) => (
                <MenuItem
                  key={PerfFilterDefault_FilterOperator[opEnum]}
                  value={opEnum}
                >
                  {OPERATOR_DISPLAY_NAMES[opEnum] ??
                    PerfFilterDefault_FilterOperator[opEnum]}
                </MenuItem>
              ))}
            </Select>
          )}

          <Autocomplete
            multiple={isMultiSelect}
            freeSolo
            size="small"
            options={isCheckboxFilter ? sortedOptions : options}
            filterOptions={(x) => x}
            disableCloseOnSelect={isMultiSelect}
            getOptionLabel={(option) => option || ''}
            renderOption={
              isMultiSelect
                ? (props, option, { selected }) => (
                    <li {...props}>
                      <Checkbox
                        style={{ marginRight: 8 }}
                        checked={selected}
                        size="small"
                      />
                      {option}
                    </li>
                  )
                : undefined
            }
            value={
              isMultiSelect
                ? [...(activeInput?.defaultValue?.values ?? [])]
                : initialValue || null
            }
            inputValue={inputValue}
            onInputChange={(_event, newInputValue) => {
              setInputValue(newInputValue);
            }}
            onChange={(_event, newValue, reason) => {
              if (isMultiSelect) {
                if (Array.isArray(newValue)) {
                  onUpdateValue(newValue);
                }
              } else {
                if (reason === 'selectOption' || reason === 'createOption') {
                  if (typeof newValue === 'string') {
                    setInputValue(newValue);
                    onUpdateValue([newValue]);
                  }
                }
              }
            }}
            onFocus={() => setIsFocused(true)}
            onBlur={() => {
              setIsFocused(false);
              handleBlur();
            }}
            loading={isLoading && isFocused}
            renderInput={(params) => (
              <TextField
                {...params}
                placeholder={
                  isCheckboxFilter
                    ? COMMON_MESSAGES.SELECT_VALUES
                    : COMMON_MESSAGES.VALUE
                }
                inputProps={{
                  ...params.inputProps,
                  'aria-label': COMMON_MESSAGES.VALUE,
                }}
                sx={{
                  ...COMPACT_TEXTFIELD_SX,
                  ...(isMultiSelect && {
                    '& .MuiInputBase-root': {
                      height: 'auto',
                      minHeight: (theme) => theme.spacing(4),
                      flexWrap: 'wrap',
                      py: 0.5,
                    },
                  }),
                }}
              />
            )}
          />
        </>
      )}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
        <Tooltip title={COMMON_MESSAGES.COPY_FILTER}>
          <IconButton
            aria-label={COMMON_MESSAGES.COPY_FILTER}
            size="small"
            sx={{ color: 'text.secondary' }}
            onClick={(e) => {
              e.stopPropagation();
              copyFilters([filter]);
            }}
          >
            <ContentCopyIcon fontSize="small" />
          </IconButton>
        </Tooltip>
        <Tooltip title={COMMON_MESSAGES.REMOVE_FILTER}>
          <IconButton
            onClick={onRemove}
            aria-label="Remove filter"
            size="small"
            sx={{ color: 'text.secondary' }}
          >
            <DeleteIcon fontSize="small" />
          </IconButton>
        </Tooltip>
      </Box>
    </Box>
  );
}
