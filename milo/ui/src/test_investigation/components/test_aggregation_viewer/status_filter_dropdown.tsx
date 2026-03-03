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

import type { SelectChangeEvent } from '@mui/material';
import {
  Typography,
  Select,
  MenuItem,
  Checkbox,
  ListItemText,
  FormControl,
  InputLabel,
  ListSubheader,
} from '@mui/material';

import { TestAggregation_ModuleStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_aggregation.pb';
import { VerdictEffectiveStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

import { useTestAggregationContext } from './context';
import {
  ALL_OPTIONS_MAP,
  formatSelectedStatuses,
  MOD_PREFIX,
  MODULE_STATUS_OPTIONS,
  TEST_PREFIX,
  TEST_STATUS_OPTIONS,
} from './status_filter_options';

export function StatusFilterDropdown() {
  const {
    selectedTestStatuses,
    setSelectedTestStatuses,
    selectedModuleStatuses,
    setSelectedModuleStatuses,
  } = useTestAggregationContext();

  const handleStatusChange = (event: SelectChangeEvent<string[]>) => {
    const {
      target: { value },
    } = event;
    const items = typeof value === 'string' ? value.split(',') : value;

    const nextTestStatuses = new Set<VerdictEffectiveStatus>();
    const nextModuleStatuses = new Set<TestAggregation_ModuleStatus>();

    for (const item of items) {
      if (item.startsWith(TEST_PREFIX)) {
        const option = ALL_OPTIONS_MAP.get(item);
        if (option) {
          nextTestStatuses.add(option.enumValue as VerdictEffectiveStatus);
        }
      } else if (item.startsWith(MOD_PREFIX)) {
        const option = ALL_OPTIONS_MAP.get(item);
        if (option) {
          nextModuleStatuses.add(
            option.enumValue as TestAggregation_ModuleStatus,
          );
        }
      }
    }

    setSelectedTestStatuses(nextTestStatuses);
    setSelectedModuleStatuses(nextModuleStatuses);
  };

  const combinedSelectedValues = [
    ...Array.from(selectedTestStatuses).map((s) => `${TEST_PREFIX}${s}`),
    ...Array.from(selectedModuleStatuses).map((s) => `${MOD_PREFIX}${s}`),
  ];

  return (
    <FormControl size="small" sx={{ width: '300px', flexShrink: 0 }}>
      <InputLabel id="status-filter-label" sx={{ fontSize: '0.875rem' }}>
        Status
      </InputLabel>
      <Select
        labelId="status-filter-label"
        id="status-select"
        multiple
        value={combinedSelectedValues}
        onChange={handleStatusChange}
        label="Status"
        sx={{ fontSize: '0.875rem' }}
        renderValue={(selected) => (
          <Typography
            variant="body2"
            sx={{
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {formatSelectedStatuses(selected)}
          </Typography>
        )}
      >
        <ListSubheader>Test status</ListSubheader>
        {TEST_STATUS_OPTIONS.map((option) => {
          const checked = selectedTestStatuses.has(option.enumValue);
          return (
            <MenuItem key={option.id} value={option.id} dense>
              <Checkbox checked={checked} size="small" />
              <ListItemText
                primary={option.label}
                primaryTypographyProps={{ variant: 'body2' }}
              />
            </MenuItem>
          );
        })}
        <ListSubheader>Module status</ListSubheader>
        {MODULE_STATUS_OPTIONS.map((option) => {
          const checked = selectedModuleStatuses.has(option.enumValue);
          return (
            <MenuItem key={option.id} value={option.id} dense>
              <Checkbox checked={checked} size="small" />
              <ListItemText
                primary={option.label}
                primaryTypographyProps={{ variant: 'body2' }}
              />
            </MenuItem>
          );
        })}
      </Select>
    </FormControl>
  );
}
