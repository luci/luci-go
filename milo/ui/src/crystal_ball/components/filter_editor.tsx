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
  Box,
  Button,
  Chip,
  IconButton,
  MenuItem,
  Select,
  SelectChangeEvent,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';

import { PerfFilter } from '@/crystal_ball/types';

// TODO: b/475638132 - Once ListMeasurementFilterColumns RPC is being used, column types are known and can be used to determine appropriate
// filter operators.
const OPERATORS = [
  'EQUAL',
  'NOT_EQUAL',
  'GREATER_THAN',
  'LESS_THAN',
  'GREATER_THAN_OR_EQUAL',
  'LESS_THAN_OR_EQUAL',
  'IN',
  'NOT_IN',
  'BETWEEN',
  'REGEX_MATCH',
  'NOT_REGEX_MATCH',
  'IS_EMPTY',
  'IS_NOT_EMPTY',
  'STARTS_WITH',
  'NOT_STARTS_WITH',
  'ENDS_WITH',
  'NOT_ENDS_WITH',
  'CONTAINS',
  'NOT_CONTAINS',
  'LIKE',
  'NOT_LIKE',
  'IS_TRUE',
  'IS_FALSE',
  'IS_NULL',
  'IS_NOT_NULL',
];

interface FilterEditorProps {
  filters: PerfFilter[];
  onUpdateFilters: (updatedFilters: PerfFilter[]) => void;
  dataSpecId: string;
  availableColumns: string[];
}

export function FilterEditor({
  filters,
  onUpdateFilters,
  dataSpecId,
  availableColumns,
}: FilterEditorProps) {
  const [expanded, setExpanded] = useState(false);
  // Local state to manage draft text inputs, keyed by filter id
  const [draftValues, setDraftValues] = useState<Record<string, string>>({});

  useEffect(() => {
    // Initialize or synchronize draftValues when the filters prop changes
    const initialDrafts: Record<string, string> = {};
    filters.forEach((filter) => {
      const key = filter.id;
      initialDrafts[key] = filter.textInput?.defaultValue?.values?.[0] || '';
    });
    setDraftValues(initialDrafts);
  }, [filters]);

  const handleAddFilter = () => {
    const newFilterId = `filter-${crypto.randomUUID()}`;
    const newFilter: PerfFilter = {
      id: newFilterId,
      column: availableColumns[0] || '',
      dataSpecId: dataSpecId,
      displayName: 'New Filter',
      textInput: {
        defaultValue: {
          values: [''],
          filterOperator: 'EQUAL',
        },
      },
    };
    onUpdateFilters([...filters, newFilter]);
    // Initialize draft value for the new filter
    setDraftValues((prev) => ({ ...prev, [newFilterId]: '' }));
  };

  const handleRemoveFilter = (index: number) => {
    const filterId = filters[index].id;
    const updatedFilters = [...filters];
    updatedFilters.splice(index, 1);
    onUpdateFilters(updatedFilters);
    // Remove from draftValues
    setDraftValues((prev) => {
      const newDrafts = { ...prev };
      delete newDrafts[filterId];
      return newDrafts;
    });
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

  const handleDefaultValueChange = (
    index: number,
    key: string,
    value: string | string[],
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

  // Update only the local draft state on each key press
  const handleDraftValueChange = (filterId: string, value: string) => {
    setDraftValues((prev) => ({ ...prev, [filterId]: value }));
  };

  // Propagate the change to the parent only when the input loses focus
  const handleDraftValueBlur = (index: number, filterId: string) => {
    const newValue = draftValues[filterId];
    const currentFilter = filters[index];
    if (currentFilter.textInput?.defaultValue?.values?.[0] !== newValue) {
      handleDefaultValueChange(index, 'values', [newValue]);
    }
  };

  const renderFilterLabel = (filter: PerfFilter) => {
    const op = filter.textInput?.defaultValue?.filterOperator || 'EQUAL';
    const val = filter.textInput?.defaultValue?.values?.[0] || '';
    return `${filter.column} ${op} "${val}"`;
  };

  return (
    <Box sx={{ mt: 1 }}>
      <Accordion expanded={expanded} onChange={() => setExpanded(!expanded)}>
        <AccordionSummary
          expandIcon={<ExpandMoreIcon />}
          aria-controls="filters-content"
          id="filters-header"
          sx={{
            '& .MuiAccordionSummary-content': {
              alignItems: 'center',
              gap: 1,
            },
          }}
        >
          <Typography variant="subtitle1">Filters</Typography>
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
          {filters.map((filter, index) => {
            const filterId = filter.id;
            return (
              <Box
                key={filterId}
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
                    handleFilterChange(index, { column: e.target.value })
                  }
                  size="small"
                  displayEmpty
                  inputProps={{ 'aria-label': 'Column' }}
                  sx={{ minWidth: 120 }}
                >
                  {filter.column &&
                    !availableColumns.includes(filter.column) && (
                      <MenuItem key={filter.column} value={filter.column}>
                        {filter.column}
                      </MenuItem>
                    )}
                  {availableColumns.map((col) => (
                    <MenuItem key={col} value={col}>
                      {col}
                    </MenuItem>
                  ))}
                </Select>

                <Select
                  value={filter.textInput?.defaultValue?.filterOperator || ''}
                  onChange={(e: SelectChangeEvent<string>) =>
                    handleDefaultValueChange(
                      index,
                      'filterOperator',
                      e.target.value,
                    )
                  }
                  size="small"
                  displayEmpty
                  inputProps={{ 'aria-label': 'Operator' }}
                  sx={{ minWidth: 120 }}
                >
                  {OPERATORS.map((op) => (
                    <MenuItem key={op} value={op}>
                      {op}
                    </MenuItem>
                  ))}
                </Select>

                <TextField
                  placeholder="Value"
                  size="small"
                  value={draftValues[filterId] || ''}
                  onChange={(e) =>
                    handleDraftValueChange(filterId, e.target.value)
                  }
                  onBlur={() => handleDraftValueBlur(index, filterId)}
                  inputProps={{ 'aria-label': 'Value' }}
                />
                <IconButton
                  onClick={() => handleRemoveFilter(index)}
                  aria-label="Remove filter"
                  color="error"
                  size="small"
                >
                  <DeleteIcon fontSize="small" />
                </IconButton>
              </Box>
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
        </AccordionDetails>
      </Accordion>
    </Box>
  );
}
