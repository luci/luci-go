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
  Delete as DeleteIcon,
  ExpandMore as ExpandMoreIcon,
} from '@mui/icons-material';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Autocomplete,
  Box,
  Button,
  Chip,
  CircularProgress,
  Divider,
  IconButton,
  MenuItem,
  Select,
  SelectChangeEvent,
  TextField,
  Typography,
} from '@mui/material';
import { useMemo, useState } from 'react';
import { useParams } from 'react-router';
import { useDebounce } from 'react-use';

import {
  ATP_TEST_NAME_COLUMN,
  AUTOCOMPLETE_DEBOUNCE_DELAY_MS,
  GLOBAL_TIME_RANGE_COLUMN,
  MAX_SUGGEST_RESULTS,
} from '@/crystal_ball/constants/api';
import {
  OPERATOR_DISPLAY_NAMES,
  TYPE_TO_OPERATORS,
} from '@/crystal_ball/constants/operators';
import { useSuggestMeasurementFilterValues } from '@/crystal_ball/hooks/use_measurement_filter_api';
import { buildFilterString } from '@/crystal_ball/utils';
import {
  MeasurementFilterColumn,
  MeasurementFilterColumn_ColumnDataType,
  measurementFilterColumn_ColumnDataTypeFromJSON,
  PerfFilter,
  PerfFilterDefault,
  PerfFilterDefault_FilterOperator,
  perfFilterDefault_FilterOperatorFromJSON,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

interface FilterEditorProps {
  title?: string;
  filters: PerfFilter[];
  globalFilters?: readonly PerfFilter[];
  onUpdateFilters: (updatedFilters: PerfFilter[]) => void;
  dataSpecId: string;
  availableColumns: readonly MeasurementFilterColumn[];
  isLoadingColumns?: boolean;
  disableAccordion?: boolean;
}

function FilterEditorRow({
  filter,
  dataSpecId,
  primaryColumns,
  secondaryColumns,
  dataType,
  onUpdateColumn,
  onUpdateOperator,
  onUpdateValue,
  onRemove,
  globalFilters,
  widgetFilters,
}: {
  filter: PerfFilter;
  dataSpecId: string;
  primaryColumns: string[];
  secondaryColumns: string[];
  dataType: MeasurementFilterColumn_ColumnDataType;
  onUpdateColumn: (column: string) => void;
  onUpdateOperator: (operator: PerfFilterDefault_FilterOperator) => void;
  onUpdateValue: (value: string) => void;
  onRemove: () => void;
  globalFilters?: readonly PerfFilter[];
  widgetFilters?: readonly PerfFilter[];
}) {
  const activeInput = filter.numberInput ?? filter.textInput;
  const initialValue = activeInput?.defaultValue?.values?.[0] ?? '';
  const [inputValue, setInputValue] = useState(initialValue);
  const [debouncedQuery, setDebouncedQuery] = useState(inputValue);
  const [isFocused, setIsFocused] = useState(false);

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
      [ATP_TEST_NAME_COLUMN, GLOBAL_TIME_RANGE_COLUMN],
      globalFilters,
      widgetFilters,
      filter.id,
    );
  }, [globalFilters, widgetFilters, filter.id]);

  const isAtpTestCol = filter.column === ATP_TEST_NAME_COLUMN;
  const hasAtpTestFilter = useMemo(() => {
    return new RegExp(`\\b${ATP_TEST_NAME_COLUMN}\\b`).test(filterString);
  }, [filterString]);
  const isEnabled = isAtpTestCol || hasAtpTestFilter;

  const { data: suggestionData, isLoading } = useSuggestMeasurementFilterValues(
    {
      parent,
      column: filter.column,
      query: debouncedQuery,
      maxResultCount: MAX_SUGGEST_RESULTS,
      filter: filterString,
    },
    {
      enabled:
        !!parent &&
        !!filter.column &&
        debouncedQuery.length > 0 &&
        isFocused &&
        isEnabled,
      retry: false,
    },
  );

  const options = suggestionData?.values ?? [];

  const handleBlur = () => {
    if (inputValue !== initialValue) {
      onUpdateValue(inputValue);
    }
  };

  return (
    <Box
      sx={{
        display: 'grid',
        gridTemplateColumns: '1fr 1fr 2fr auto',
        gap: 1,
        alignItems: 'center',
        mb: 1.5,
      }}
    >
      <Select
        value={filter.column}
        onChange={(e: SelectChangeEvent<string>) =>
          onUpdateColumn(e.target.value)
        }
        size="small"
        displayEmpty
        inputProps={{ 'aria-label': 'Column' }}
        sx={{ minWidth: 120 }}
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
        inputProps={{ 'aria-label': 'Operator' }}
        sx={{ minWidth: 120 }}
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

      <Autocomplete
        freeSolo
        size="small"
        options={options}
        filterOptions={(x) => x}
        value={initialValue || null}
        inputValue={inputValue}
        onInputChange={(_event, newInputValue) => {
          setInputValue(newInputValue);
        }}
        onChange={(_event, newValue, reason) => {
          if (reason === 'selectOption' || reason === 'createOption') {
            if (typeof newValue === 'string') {
              setInputValue(newValue);
              onUpdateValue(newValue);
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
            placeholder="Value"
            inputProps={{
              ...params.inputProps,
              'aria-label': 'Value',
            }}
          />
        )}
      />
      <IconButton
        onClick={onRemove}
        aria-label="Remove filter"
        color="error"
        size="small"
      >
        <DeleteIcon fontSize="small" />
      </IconButton>
    </Box>
  );
}

export function FilterEditor({
  title,
  filters,
  globalFilters,
  onUpdateFilters,
  dataSpecId,
  availableColumns,
  isLoadingColumns,
  disableAccordion = false,
}: FilterEditorProps) {
  const [expanded, setExpanded] = useState(false);

  const handleAddFilter = () => {
    const newFilterId = `filter-${crypto.randomUUID()}`;
    const selectedColumn = availableColumns[0];
    const isNumber =
      selectedColumn?.dataType ===
        MeasurementFilterColumn_ColumnDataType.INT64 ||
      selectedColumn?.dataType ===
        MeasurementFilterColumn_ColumnDataType.DOUBLE;

    const newFilter: PerfFilter = {
      id: newFilterId,
      column: selectedColumn?.column ?? '',
      dataSpecId: dataSpecId,
      displayName: 'New Filter',
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
                onUpdateValue={(value) =>
                  handleDefaultValueChange(index, 'values', [value])
                }
                onRemove={() => handleRemoveFilter(index)}
                globalFilters={globalFilters}
                widgetFilters={filters}
              />
            );
          })}
          <Button
            startIcon={<AddIcon />}
            onClick={handleAddFilter}
            variant="outlined"
            size="small"
            sx={{ mt: 1 }}
          >
            Add Filter
          </Button>
        </>
      )}
    </>
  );

  if (disableAccordion) {
    return (
      <Box sx={{ mt: 1 }}>
        {title && (
          <Typography variant="subtitle1" sx={{ mb: 1 }}>
            {title}
          </Typography>
        )}
        {content}
      </Box>
    );
  }

  return (
    <Box sx={{ mt: 1 }}>
      <Accordion
        expanded={expanded}
        onChange={() => setExpanded(!expanded)}
        disableGutters
      >
        <AccordionSummary
          expandIcon={<ExpandMoreIcon />}
          aria-controls="filters-content"
          id="filters-header"
          sx={{
            '& .MuiAccordionSummary-content': {
              alignItems: 'baseline',
              gap: 1,
              margin: '12px 0',
              '&.Mui-expanded': {
                margin: '12px 0',
              },
            },
          }}
        >
          <Typography variant="subtitle1" sx={{ flexShrink: 0 }}>
            {title ?? 'Filters'}
          </Typography>
          {!expanded && filters.length > 0 && (
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
              {filters.map((filter) => (
                <Chip
                  key={filter.id}
                  label={renderFilterLabel(filter)}
                  size="small"
                />
              ))}
            </Box>
          )}
          {!expanded && filters.length === 0 && (
            <Typography variant="body2" color="text.secondary">
              No filters applied.
            </Typography>
          )}
        </AccordionSummary>
        <AccordionDetails>{content}</AccordionDetails>
      </Accordion>
    </Box>
  );
}
