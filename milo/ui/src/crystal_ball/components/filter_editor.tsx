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
  AUTOCOMPLETE_DEBOUNCE_DELAY_MS,
  MAX_SUGGEST_RESULTS,
} from '@/crystal_ball/constants/api';
import { useSuggestMeasurementFilterValues } from '@/crystal_ball/hooks/use_measurement_filter_api';
import {
  MeasurementFilterColumn,
  PerfFilter,
  PerfFilterDefault,
  PerfFilterDefault_FilterOperator,
  perfFilterDefault_FilterOperatorFromJSON,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

// TODO: b/475638132 - Once ListMeasurementFilterColumns RPC is being used, column types are known and can be used to determine appropriate
// filter operators.
const OPERATORS = Object.keys(PerfFilterDefault_FilterOperator).filter(
  (key): key is keyof typeof PerfFilterDefault_FilterOperator =>
    isNaN(Number(key)) && key !== 'FILTER_OPERATOR_UNSPECIFIED',
);

interface FilterEditorProps {
  title?: string;
  filters: PerfFilter[];
  onUpdateFilters: (updatedFilters: PerfFilter[]) => void;
  dataSpecId: string;
  availableColumns: readonly MeasurementFilterColumn[];
  isLoadingColumns?: boolean;
}

function FilterEditorRow({
  filter,
  dataSpecId,
  primaryColumns,
  secondaryColumns,
  onUpdateColumn,
  onUpdateOperator,
  onUpdateValue,
  onRemove,
}: {
  filter: PerfFilter;
  dataSpecId: string;
  primaryColumns: string[];
  secondaryColumns: string[];
  onUpdateColumn: (column: string) => void;
  onUpdateOperator: (operator: PerfFilterDefault_FilterOperator) => void;
  onUpdateValue: (value: string) => void;
  onRemove: () => void;
}) {
  const initialValue = filter.textInput?.defaultValue?.values?.[0] ?? '';
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

  const { data: suggestionData, isLoading } = useSuggestMeasurementFilterValues(
    {
      parent,
      column: filter.column,
      query: debouncedQuery,
      maxResultCount: MAX_SUGGEST_RESULTS,
    },
    {
      enabled:
        !!parent && !!filter.column && debouncedQuery.length > 0 && isFocused,
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
          filter.textInput?.defaultValue?.filterOperator !== undefined
            ? perfFilterDefault_FilterOperatorFromJSON(
                filter.textInput.defaultValue.filterOperator,
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
        {OPERATORS.map((op) => (
          <MenuItem key={op} value={PerfFilterDefault_FilterOperator[op]}>
            {op}
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
  onUpdateFilters,
  dataSpecId,
  availableColumns,
  isLoadingColumns,
}: FilterEditorProps) {
  const [expanded, setExpanded] = useState(false);

  const handleAddFilter = () => {
    const newFilterId = `filter-${crypto.randomUUID()}`;
    const newFilter: PerfFilter = {
      id: newFilterId,
      column: availableColumns[0]?.column ?? '',
      dataSpecId: dataSpecId,
      displayName: 'New Filter',
      textInput: {
        defaultValue: {
          values: [''],
          filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
        },
      },
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

    if (currentFilter.textInput?.defaultValue) {
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
    return `${filter.column} ${PerfFilterDefault_FilterOperator[op]} "${val}"`;
  };

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
              alignItems: filters.length === 0 ? 'baseline' : 'center',
              gap: 1,
            },
          }}
        >
          <Typography variant="subtitle1">{title ?? 'Filters'}</Typography>
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
        <AccordionDetails>
          {isLoadingColumns ? (
            <Box sx={{ display: 'flex', justifySelf: 'center', p: 2 }}>
              <CircularProgress size={24} />
            </Box>
          ) : (
            <>
              {filters.map((filter, index) => (
                <FilterEditorRow
                  key={filter.id}
                  filter={filter}
                  dataSpecId={dataSpecId}
                  primaryColumns={primaryColumns}
                  secondaryColumns={secondaryColumns}
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
                />
              ))}
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
        </AccordionDetails>
      </Accordion>
    </Box>
  );
}
