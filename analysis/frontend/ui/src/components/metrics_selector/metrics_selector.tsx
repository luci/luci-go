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

import {
  Checkbox,
  ListItemText,
  MenuItem,
  OutlinedInput,
  Select,
  SelectChangeEvent,
} from '@mui/material';

import { MetricId } from '@/legacy_services/shared_models';
import { Metric } from '@/legacy_services/metrics';

const MenuProps = {
  PaperProps: {
    style: {
      maxHeight: 500,
      width: 250,
    },
  },
};

interface Props {
    metrics: Metric[];
    selectedMetrics: MetricId[];
    handleSelectedMetricsChanged: (selectedMetricsIds: MetricId[]) => void;
    labelId?: string;
}

const MetricsSelector = ({
  metrics,
  selectedMetrics,
  handleSelectedMetricsChanged,
  labelId,
}: Props) => {
  function selectionChanged(event: SelectChangeEvent<string[]>) {
    const {
      target: { value },
    } = event;
    // On autofill we get a stringified value.
    const selectedMetricsValue = typeof value === 'string' ? value.split(',') : value;

    handleSelectedMetricsChanged(selectedMetricsValue);
  }

  function renderValue(selected: string[]) {
    return metrics.filter((m) => selected.indexOf(m.metricId) >= 0)
        .map((m) => m.humanReadableName).join(', ');
  }
  return (
    <Select
      labelId={labelId}
      multiple
      value={selectedMetrics}
      onChange={selectionChanged}
      input={<OutlinedInput label="Metrics" />}
      renderValue={renderValue}
      MenuProps={MenuProps}
      inputProps={{
        'data-testid': 'metrics-selector',
      }}
    >
      {metrics.map((metric) => (
        <MenuItem key={metric.metricId} value={metric.metricId} sx={{
          whiteSpace: 'unset',
          wordBreak: 'break-word',
        }}>
          <Checkbox checked={selectedMetrics.indexOf(metric.metricId) > -1} />
          <ListItemText primary={metric.humanReadableName} secondary={metric.description} />
        </MenuItem>
      ))}
    </Select>
  );
};

export default MetricsSelector;
