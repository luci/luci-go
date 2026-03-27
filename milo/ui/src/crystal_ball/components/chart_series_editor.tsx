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
  IconButton,
  TextField,
  Typography,
} from '@mui/material';
import { useCallback, useMemo, useState } from 'react';
import { useParams } from 'react-router';
import { useDebounce } from 'react-use';

import { FilterEditor } from '@/crystal_ball/components';
import {
  ATP_TEST_NAME_COLUMN,
  AUTOCOMPLETE_DEBOUNCE_DELAY_MS,
  GLOBAL_TIME_RANGE_COLUMN,
  MAX_SUGGEST_RESULTS,
} from '@/crystal_ball/constants';
import { useSuggestMeasurementFilterValues } from '@/crystal_ball/hooks/use_measurement_filter_api';
import {
  buildFilterString,
  isStringArray,
  generateColor,
} from '@/crystal_ball/utils';
import {
  MeasurementFilterColumn,
  MeasurementFilterColumn_FilterScope,
  PerfChartSeries,
  PerfFilter,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

interface ChartSeriesEditorProps {
  series: PerfChartSeries[];
  onUpdateSeries: (updatedSeries: PerfChartSeries[]) => void;
  dataSpecId: string;
  globalFilters?: readonly PerfFilter[];
  widgetFilters?: readonly PerfFilter[];
  filterColumns: readonly MeasurementFilterColumn[];
  isLoadingFilterColumns?: boolean;
}

const getSeriesId = (s: PerfChartSeries, index: number) =>
  s.displayName || `series-${index}`;

export function ChartSeriesEditor({
  series,
  onUpdateSeries,
  dataSpecId,
  globalFilters,
  widgetFilters,
  filterColumns,
  isLoadingFilterColumns,
}: ChartSeriesEditorProps) {
  const handleAddSeries = () => {
    const newSeries: PerfChartSeries = PerfChartSeries.fromPartial({
      displayName: `series-${crypto.randomUUID()}`,
      metricField: '',
      dataSpecId: dataSpecId,
      color: generateColor(series.length),
      filters: [],
    });
    onUpdateSeries([...series, newSeries]);
  };

  const handleUpdateSeriesItem = useCallback(
    (index: number, updatedItem: PerfChartSeries) => {
      const updatedSeries = [...series];
      updatedSeries[index] = updatedItem;
      onUpdateSeries(updatedSeries);
    },
    [series, onUpdateSeries],
  );

  const handleRemoveSeries = useCallback(
    (index: number) => {
      const updatedSeries = [...series];
      updatedSeries.splice(index, 1);
      onUpdateSeries(updatedSeries);
    },
    [series, onUpdateSeries],
  );

  const metricFilterColumns = useMemo(
    () =>
      filterColumns.filter(
        (c) =>
          c.applicableScopes?.includes(
            MeasurementFilterColumn_FilterScope.METRIC,
          ) ||
          (isStringArray(c.applicableScopes) &&
            c.applicableScopes.includes('METRIC')),
      ),
    [filterColumns],
  );

  return (
    <Box sx={{ mt: 1 }}>
      <Typography variant="subtitle1" sx={{ mb: 1 }}>
        Series
      </Typography>
      {series.map((s, index) => (
        <ChartSeriesItem
          key={getSeriesId(s, index)}
          series={s}
          onUpdate={(updatedItem) => handleUpdateSeriesItem(index, updatedItem)}
          onRemove={() => handleRemoveSeries(index)}
          dataSpecId={dataSpecId}
          globalFilters={globalFilters}
          widgetFilters={widgetFilters}
          metricFilterColumns={metricFilterColumns}
          isLoadingColumns={isLoadingFilterColumns}
        />
      ))}
      <Button
        startIcon={<AddIcon />}
        onClick={handleAddSeries}
        variant="outlined"
        size="small"
        sx={{ mt: 1 }}
      >
        Add Series
      </Button>
    </Box>
  );
}

interface ChartSeriesItemProps {
  series: PerfChartSeries;
  onUpdate: (updatedSeries: PerfChartSeries) => void;
  onRemove: () => void;
  dataSpecId: string;
  globalFilters?: readonly PerfFilter[];
  widgetFilters?: readonly PerfFilter[];
  metricFilterColumns: readonly MeasurementFilterColumn[];
  isLoadingColumns?: boolean;
}

function ChartSeriesItem({
  series,
  onUpdate,
  onRemove,
  dataSpecId,
  globalFilters,
  widgetFilters,
  metricFilterColumns,
  isLoadingColumns,
}: ChartSeriesItemProps) {
  const [expanded, setExpanded] = useState(false);
  const [displayName, setDisplayName] = useState(series.displayName ?? '');
  const [color, setColor] = useState(series.color ?? '');
  const [inputValue, setInputValue] = useState(series.metricField ?? '');
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
    );
  }, [globalFilters, widgetFilters]);

  const hasAtpTestFilter = useMemo(() => {
    return new RegExp(`\\b${ATP_TEST_NAME_COLUMN}\\b`).test(filterString);
  }, [filterString]);

  const { data: suggestionData, isLoading: isLoadingSuggestions } =
    useSuggestMeasurementFilterValues(
      {
        parent,
        column: 'metric_key',
        query: debouncedQuery,
        maxResultCount: MAX_SUGGEST_RESULTS,
        filter: filterString,
      },
      {
        enabled:
          !!parent &&
          debouncedQuery.length > 0 &&
          isFocused &&
          hasAtpTestFilter,
        retry: false,
      },
    );

  const options = suggestionData?.values || [];

  const handleBlurDisplayName = () => {
    if (displayName !== series.displayName) {
      onUpdate({ ...series, displayName });
    }
  };

  const handleBlurColor = () => {
    if (color !== series.color) {
      onUpdate({ ...series, color });
    }
  };

  const handleMetricFieldChange = (newValue: string) => {
    setInputValue(newValue);
    onUpdate({
      ...series,
      metricField: newValue,
      displayName:
        !series.displayName || series.displayName.startsWith('series-')
          ? newValue || series.displayName
          : series.displayName,
    });
  };

  const handleUpdateFilters = (updatedFilters: PerfFilter[]) => {
    onUpdate({ ...series, filters: updatedFilters });
  };

  return (
    <Accordion
      expanded={expanded}
      onChange={() => setExpanded(!expanded)}
      disableGutters
      sx={{ mb: 1, border: '1px solid', borderColor: 'divider' }}
    >
      <AccordionSummary
        expandIcon={<ExpandMoreIcon />}
        aria-controls={`series-${series.displayName}-content`}
        id={`series-${series.displayName}-header`}
        sx={{
          '& .MuiAccordionSummary-content': {
            alignItems: 'center',
            gap: 1,
          },
        }}
      >
        <Box
          sx={{
            width: 16,
            height: 16,
            bgcolor: series.color || color,
            borderRadius: '50%',
            border: '1px solid',
            borderColor: 'divider',
          }}
        />
        <Typography>{series.displayName || 'Untitled Series'}</Typography>
        {!expanded && series.metricField && (
          <Typography variant="body2" color="text.secondary" sx={{ ml: 1 }}>
            ({series.metricField})
          </Typography>
        )}
      </AccordionSummary>
      <AccordionDetails>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: '1fr 60px auto',
              gap: 1,
              alignItems: 'center',
            }}
          >
            <TextField
              label="Display Name"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              onBlur={handleBlurDisplayName}
              size="small"
              fullWidth
            />
            <TextField
              label="Color"
              value={color}
              onChange={(e) => setColor(e.target.value)}
              onBlur={handleBlurColor}
              size="small"
              fullWidth
              inputProps={{
                type: 'color',
                sx: { padding: '2px', height: '40px', cursor: 'pointer' },
              }}
            />
            <IconButton
              onClick={onRemove}
              aria-label="Remove series"
              color="error"
              size="small"
            >
              <DeleteIcon fontSize="small" />
            </IconButton>
          </Box>

          <Autocomplete
            freeSolo
            size="small"
            options={options}
            filterOptions={(x) => x}
            value={series.metricField || null}
            inputValue={inputValue}
            onInputChange={(_event, newInputValue) => {
              setInputValue(newInputValue);
            }}
            onChange={(_event, newValue, reason) => {
              if (reason === 'selectOption' || reason === 'createOption') {
                if (typeof newValue === 'string') {
                  handleMetricFieldChange(newValue);
                }
              }
            }}
            onFocus={() => setIsFocused(true)}
            onBlur={() => {
              setIsFocused(false);
              if (inputValue !== series.metricField) {
                handleMetricFieldChange(inputValue);
              }
            }}
            loading={isLoadingSuggestions && isFocused}
            renderInput={(params) => (
              <TextField
                {...params}
                label="Metric Field"
                placeholder="e.g., MemAvailable_CacheProcDirty_bytes"
                inputProps={{
                  ...params.inputProps,
                  'aria-label': 'Metric Field',
                }}
              />
            )}
          />

          <FilterEditor
            title="Series Filters"
            filters={[...(series.filters || [])]}
            onUpdateFilters={handleUpdateFilters}
            dataSpecId={dataSpecId}
            availableColumns={metricFilterColumns}
            isLoadingColumns={isLoadingColumns}
            disableAccordion={true}
          />
        </Box>
      </AccordionDetails>
    </Accordion>
  );
}
