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

import {
  Checkbox,
  FormControl,
  Grid,
  InputLabel,
  ListItemText,
  MenuItem,
  OutlinedInput,
  Select,
  SelectChangeEvent,
} from '@mui/material';

import { ClusterTableContextData } from '@/components/clusters_table/clusters_table_context';
import { useSelectedMetricsParam } from '@/components/clusters_table/hooks';

const ITEM_HEIGHT = 48;
const ITEM_PADDING_TOP = 8;
const MenuProps = {
  PaperProps: {
    style: {
      maxHeight: ITEM_HEIGHT * 4.5 + ITEM_PADDING_TOP,
      width: 250,
    },
  },
};

const ClustersTableMetricSelection = () => {
  const metrics = useContext(ClusterTableContextData).metrics;
  const [selectedMetrics, updateSelectedMetricsParam] = useSelectedMetricsParam(metrics);

  function handleSelectedMetricsChanged(event: SelectChangeEvent<string[]>) {
    const {
      target: { value },
    } = event;
    // On autofill we get a stringified value.
    const selectedMetricsValue = typeof value === 'string' ? value.split(',') : value;

    // Only update if at least one metric has been selected.
    if (selectedMetricsValue.length > 0) {
      updateSelectedMetricsParam(metrics.filter((m) => selectedMetricsValue.indexOf(m.metricId) > -1));
    }
  }

  function renderValue(selected: string[]) {
    return metrics.filter((m) => selected.indexOf(m.metricId) >= 0)
        .map((m) => m.humanReadableName).join(', ');
  }
  return (
    <Grid item>
      <FormControl
        data-testid='metrics-selection'
        sx={{
          width: '100%',
        }}>
        <InputLabel id="metrics-selection-label">Metrics</InputLabel>
        <Select
          labelId="metrics-selection-label"
          id="metrics-selection"
          multiple
          value={selectedMetrics.map((m) => m.metricId)}
          onChange={handleSelectedMetricsChanged}
          input={<OutlinedInput label="Metrics" />}
          renderValue={renderValue}
          MenuProps={MenuProps}
          inputProps={{ 'data-testid': 'clusters-table-metrics-selection' }}
        >
          {metrics.map((metric) => (
            <MenuItem key={metric.metricId} value={metric.metricId}>
              <Checkbox checked={selectedMetrics.indexOf(metric) > -1} />
              <ListItemText primary={metric.humanReadableName} />
            </MenuItem>
          ))}
        </Select>
      </FormControl>
    </Grid>
  );
};

export default ClustersTableMetricSelection;
