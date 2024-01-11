// Copyright 2022 The LUCI Authors.
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

import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import FormControl from '@mui/material/FormControl';
import Grid from '@mui/material/Grid';
import InputLabel from '@mui/material/InputLabel';
import MenuItem from '@mui/material/MenuItem';
import OutlinedInput from '@mui/material/OutlinedInput';
import Select, { SelectChangeEvent } from '@mui/material/Select';

import {
  ImpactFilter,
  ImpactFilters,
  VariantGroup,
} from '@/tools/failures_tools';
import { Metric } from '@/legacy_services/metrics';

interface Props {
    metrics: Metric[],
    metricFilter: Metric | undefined,
    onMetricFilterChanged: (event: SelectChangeEvent) => void,
    impactFilter: ImpactFilter,
    onImpactFilterChanged: (event: SelectChangeEvent) => void,
    variantGroups: VariantGroup[],
    selectedVariantGroups: string[],
    handleVariantGroupsChange: (event: SelectChangeEvent<string[]>) => void,
}

const FailuresTableFilter = ({
  metrics,
  metricFilter,
  onMetricFilterChanged,
  impactFilter,
  onImpactFilterChanged,
  variantGroups,
  selectedVariantGroups,
  handleVariantGroupsChange,
}: Props) => {
  return (
    <>
      <Grid container data-testid="failure_table_filter" sx={{ paddingTop: '8px', paddingBottom: '12px' }}>
        <Grid item xs={3} sx={{ paddingRight: '1rem' }}>
          <FormControl fullWidth data-testid="failure_filter">
            <InputLabel id="failure_filter_label">Failure filter</InputLabel>
            <Select
              labelId="failure_filter_label"
              id="failure_filter"
              value={metricFilter?.metricId || ''}
              label="Failure filter"
              onChange={onMetricFilterChanged}
              inputProps={{ 'data-testid': 'failure_filter_input' }}
              renderValue={(value) => <strong>Only {metrics.find((m) => m.metricId == value)?.humanReadableName}</strong>}
            >
              <MenuItem value="">None</MenuItem>
              {
                metrics.map((metric) => (
                  <MenuItem key={metric.metricId} value={metric.metricId}>Only {metric.humanReadableName}</MenuItem>
                ))
              }
            </Select>
          </FormControl>
        </Grid>
        <Grid item xs={3} sx={{ paddingRight: '1rem' }}>
          <FormControl fullWidth data-testid="impact_filter">
            <InputLabel id="impact_filter_label">Impact filter</InputLabel>
            <Select
              labelId="impact_filter_label"
              id="impact_filter"
              value={impactFilter.id}
              label="Impact filter"
              onChange={onImpactFilterChanged}
              inputProps={{ 'data-testid': 'impact_filter_input' }}>
              {
                ImpactFilters.map((filter) => (
                  <MenuItem key={filter.id} value={filter.id}>{filter.name}</MenuItem>
                ))
              }
            </Select>
          </FormControl>
        </Grid>
        <Grid item xs={6}>
          <FormControl fullWidth data-testid="group_by">
            <InputLabel id="group_by_label">Group by</InputLabel>
            <Select
              labelId="group_by_label"
              id="group_by"
              multiple
              // Limit to showing variant groups that actually exist.
              value={selectedVariantGroups.filter((key) => variantGroups.some((vg) => vg.key == key))}
              onChange={handleVariantGroupsChange}
              input={<OutlinedInput id="group_by_select" label="Group by" />}
              renderValue={(selected) => (
                <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
                  {selected.map((value) => (
                    <Chip key={value} label={value} />
                  ))}
                </Box>
              )}
              inputProps={{ 'data-testid': 'group_by_input' }}>
              {variantGroups.map((variantGroup) => (
                <MenuItem
                  key={variantGroup.key}
                  value={variantGroup.key}>
                  {variantGroup.key} ({variantGroup.values.length})
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Grid>
      </Grid>
    </>
  );
};

export default FailuresTableFilter;
