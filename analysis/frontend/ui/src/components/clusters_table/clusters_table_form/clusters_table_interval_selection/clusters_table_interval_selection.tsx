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
  FormControl,
  Grid,
  InputLabel,
  MenuItem,
  OutlinedInput,
  Select,
  SelectChangeEvent,
} from '@mui/material';

import { useIntervalParam } from '@/components/clusters_table/hooks';
import { TimeInterval } from '@/hooks/use_fetch_clusters';

const ITEM_HEIGHT = 48;
const ITEM_PADDING_TOP = 8;
const MenuProps = {
  PaperProps: {
    style: {
      maxHeight: ITEM_HEIGHT * 4.5 + ITEM_PADDING_TOP,
      width: 100,
    },
  },
};

// The time interval options from which to select.
// Note: the ClustersTable component uses the first entry as the default
// selection.
export const TIME_INTERVAL_OPTIONS: TimeInterval[] = [
  {
    id: '24h',
    label: 'Last 24 hours',
    duration: 24,
  },
  {
    id: '3d',
    label: 'Last 3 days',
    duration: 3 * 24,
  },
  {
    id: '7d',
    label: 'Last 7 days',
    duration: 7 * 24,
  },
];

export const ClustersTableIntervalSelection = () => {
  const [selectedInterval, updateIntervalParam] = useIntervalParam(TIME_INTERVAL_OPTIONS);

  function handleIntervalChanged(event: SelectChangeEvent) {
    const {
      target: { value },
    } = event;
    const newInterval = TIME_INTERVAL_OPTIONS.find((interval) => interval.id === value);
    if (newInterval) {
      updateIntervalParam(newInterval);
    }
  }

  return (
    <Grid item>
      <FormControl
        data-testid="interval-selection"
        sx={{ width: '100%' }}>
        <InputLabel id="interval-selection-label">Time range</InputLabel>
        <Select
          labelId="interval-selection-label"
          id="interval-selection"
          value={selectedInterval ? selectedInterval.id : ''}
          onChange={handleIntervalChanged}
          input={<OutlinedInput label="Time range" />}
          MenuProps={MenuProps}
          inputProps={{ 'data-testid': 'clusters-table-interval-selection' }}
        >
          {TIME_INTERVAL_OPTIONS.map((interval) => (
            <MenuItem key={interval.id} value={interval.id}>
              {interval.label}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
    </Grid>
  );
};
