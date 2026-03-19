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
  IconButton,
  TextField,
  Typography,
} from '@mui/material';
import { useCallback, useState } from 'react';
import { useParams } from 'react-router';
import { useDebounce } from 'react-use';

import {
  AUTOCOMPLETE_DEBOUNCE_DELAY_MS,
  MAX_SUGGEST_RESULTS,
} from '@/crystal_ball/constants';
import { useSuggestMeasurementFilterValues } from '@/crystal_ball/hooks/use_measurement_filter_api';
import { PerfChartSeries } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

interface ChartSeriesEditorProps {
  series: PerfChartSeries[];
  onUpdateSeries: (updatedSeries: PerfChartSeries[]) => void;
  dataSpecId: string;
}

// Helper function to get a stable ID for a series item
const getSeriesId = (s: PerfChartSeries, index: number) =>
  s.displayName || `series-${index}`;

function ChartSeriesEditorRow({
  singleSeries,
  dataSpecId,
  onUpdate,
  onRemove,
}: {
  singleSeries: PerfChartSeries;
  dataSpecId: string;
  onUpdate: (metricField: string) => void;
  onRemove: () => void;
}) {
  const [inputValue, setInputValue] = useState(singleSeries.metricField || '');
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
      column: 'metric_key',
      query: debouncedQuery,
      maxResultCount: MAX_SUGGEST_RESULTS,
    },
    {
      enabled: !!parent && debouncedQuery.length > 0 && isFocused,
      retry: false,
    },
  );

  // The API returns an object containing `values`
  const options = suggestionData?.values || [];

  const handleBlur = () => {
    onUpdate(inputValue);
  };

  return (
    <Box
      sx={{
        display: 'grid',
        gridTemplateColumns: '1fr auto',
        gap: 1,
        alignItems: 'center',
        mb: 1.5,
      }}
    >
      <Autocomplete
        freeSolo
        size="small"
        options={options}
        filterOptions={(x) => x}
        value={singleSeries.metricField || null}
        inputValue={inputValue}
        onInputChange={(_event, newInputValue) => {
          setInputValue(newInputValue);
        }}
        onChange={(_event, newValue, reason) => {
          if (reason === 'selectOption' || reason === 'createOption') {
            if (typeof newValue === 'string') {
              setInputValue(newValue);
              onUpdate(newValue);
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
            placeholder="Metric Field (e.g., MemAvailable_CacheProcDirty_bytes)"
            inputProps={{
              ...params.inputProps,
              'aria-label': 'Metric Field',
            }}
          />
        )}
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
  );
}

export function ChartSeriesEditor({
  series,
  onUpdateSeries,
  dataSpecId,
}: ChartSeriesEditorProps) {
  const [expanded, setExpanded] = useState(false);

  const handleAddSeries = () => {
    const newSeries: PerfChartSeries = PerfChartSeries.fromPartial({
      displayName: `series-${crypto.randomUUID()}`,
      metricField: '',
      dataSpecId: dataSpecId,
    });
    onUpdateSeries([...series, newSeries]);
  };

  const handleRemoveSeries = (index: number) => {
    const updatedSeries = [...series];
    updatedSeries.splice(index, 1);
    onUpdateSeries(updatedSeries);
  };

  const handleUpdateSingleSeries = useCallback(
    (index: number, newMetricField: string) => {
      const currentSeries = series[index];
      if (currentSeries && currentSeries.metricField !== newMetricField) {
        const updatedSeries = [...series];
        updatedSeries[index] = {
          ...currentSeries,
          metricField: newMetricField,
          displayName:
            currentSeries.displayName &&
            !currentSeries.displayName.startsWith('series-')
              ? currentSeries.displayName
              : newMetricField || currentSeries.displayName,
        };
        onUpdateSeries(updatedSeries);
      }
    },
    [series, onUpdateSeries],
  );

  return (
    <Box sx={{ mt: 1 }}>
      <Accordion expanded={expanded} onChange={() => setExpanded(!expanded)}>
        <AccordionSummary
          expandIcon={<ExpandMoreIcon />}
          aria-controls="series-content"
          id="series-header"
          sx={{
            '& .MuiAccordionSummary-content': {
              alignItems: 'center',
              gap: 1,
            },
          }}
        >
          <Typography variant="subtitle1">Series</Typography>
          {!expanded && series.length > 0 && (
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
              {series.map((s, index) => (
                <Chip
                  key={getSeriesId(s, index)}
                  label={s.metricField || 'New Series'}
                  size="small"
                />
              ))}
            </Box>
          )}
          {!expanded && series.length === 0 && (
            <Typography variant="body2" color="text.secondary">
              No series added.
            </Typography>
          )}
        </AccordionSummary>
        <AccordionDetails>
          {series.map((singleSeries, index) => {
            const seriesId = getSeriesId(singleSeries, index);
            return (
              <ChartSeriesEditorRow
                key={seriesId}
                singleSeries={singleSeries}
                dataSpecId={dataSpecId}
                onUpdate={(newMetricField) =>
                  handleUpdateSingleSeries(index, newMetricField)
                }
                onRemove={() => handleRemoveSeries(index)}
              />
            );
          })}
          <Button
            startIcon={<AddIcon />}
            onClick={handleAddSeries}
            variant="outlined"
            size="small"
            sx={{ mt: 1 }}
          >
            Add Series
          </Button>
        </AccordionDetails>
      </Accordion>
    </Box>
  );
}
