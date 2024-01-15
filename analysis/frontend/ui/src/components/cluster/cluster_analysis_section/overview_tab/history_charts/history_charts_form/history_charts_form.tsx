// Copyright 2023 The LUCI Authors.
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

import { useContext } from 'react';

import Box from '@mui/material/Box';
import FormControl from '@mui/material/FormControl';
import FormControlLabel from '@mui/material/FormControlLabel';
import InputLabel from '@mui/material/InputLabel';
import MenuItem from '@mui/material/MenuItem';
import OutlinedInput from '@mui/material/OutlinedInput';
import Select, { SelectChangeEvent } from '@mui/material/Select';
import Switch from '@mui/material/Switch';
import Typography from '@mui/material/Typography';

import PanelHeading from '@/components/headings/panel_heading/panel_heading';
import MetricsSelector from '@/components/metrics_selector/metrics_selector';
import { MetricId } from '@/types/metric_id';

import { OverviewTabContextData } from '../../overview_tab_context';
import {
  HistoryTimeRange,
  useAnnotatedParam,
  useHistoryTimeRangeParam,
  useSelectedMetricsParam,
} from '../hooks';

const ITEM_HEIGHT = 66;
const ITEM_PADDING_TOP = 8;
const MenuProps = {
  PaperProps: {
    style: {
      maxHeight: ITEM_HEIGHT * 4.5 + ITEM_PADDING_TOP,
      width: 250,
    },
  },
};

export const HISTORY_TIME_RANGE_OPTIONS: HistoryTimeRange[] = [
  {
    id: '30d',
    label: '30 days',
    value: 30,
  },
  {
    id: '90d',
    label: '90 days',
    value: 90,
  },
];

export const HistoryChartsForm = () => {
  const { metrics } = useContext(OverviewTabContextData);

  const [isAnnotated, updateAnnotatedParam] = useAnnotatedParam();
  const [selectedHistoryTimeRange, updateHistoryTimeRangeParam] = useHistoryTimeRangeParam(HISTORY_TIME_RANGE_OPTIONS);
  const [selectedMetrics, updateSelectedMetricsParam] = useSelectedMetricsParam(metrics);

  const handleAnnotationsChange = () => {
    const current = isAnnotated || false;
    updateAnnotatedParam(!current);
  };

  function handleHistoryTimeRangeChanged(event: SelectChangeEvent) {
    const {
      target: { value },
    } = event;
    const newHistoryTimeRange = HISTORY_TIME_RANGE_OPTIONS.find((option) => option.id === value);
    if (newHistoryTimeRange) {
      updateHistoryTimeRangeParam(newHistoryTimeRange);
    }
  }

  function handleSelectedMetricsChanged(selectedMetrics: MetricId[]) {
    // Only update if at least one metric has been selected.
    if (selectedMetrics.length > 0) {
      updateSelectedMetricsParam(metrics.filter((m) => selectedMetrics.indexOf(m.metricId) > -1));
    }
  }

  return (
    <Box sx={{
      display: 'flex',
      alignItems: 'baseline',
      gap: '1rem',
    }} >
      <PanelHeading>History</PanelHeading>
      <Typography color="GrayText">All dates and times are in UTC.</Typography>
      <div style={{ flexGrow: 1 }}></div>
      <FormControlLabel
        control={
          <Switch
            checked={isAnnotated || false}
            onChange={handleAnnotationsChange}
            inputProps={{
              'aria-label': 'toggle annotations',
            }} />
        }
        label="Annotate values"
        labelPlacement="end"
      />
      <FormControl sx={{ m: 1, width: 300 }}>
        <InputLabel id="time-range-label">Time Range</InputLabel>
        <Select
          labelId="time-range-label"
          value={selectedHistoryTimeRange?.id || ''}
          onChange={handleHistoryTimeRangeChanged}
          input={<OutlinedInput label="Time Range" />}
          MenuProps={MenuProps}
          inputProps={{
            'aria-label': 'select time range',
            'data-testid': 'history-charts-form-time-range-selection',
          }}
        >
          {HISTORY_TIME_RANGE_OPTIONS.map((option) =>
            <MenuItem
              key={option.id}
              value={option.id} >
              {option.label}
            </MenuItem>,
          )}
        </Select>
      </FormControl>

      <FormControl sx={{ m: 1, width: '30%' }}>
        <InputLabel id="metric-label">Metrics</InputLabel>
        <MetricsSelector labelId="metric-label" metrics={metrics} selectedMetrics={selectedMetrics.map((m) => m.metricId)} handleSelectedMetricsChanged={handleSelectedMetricsChanged} />
      </FormControl>
    </Box>
  );
};
