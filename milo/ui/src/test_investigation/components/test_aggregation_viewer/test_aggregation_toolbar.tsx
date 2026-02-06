// Copyright 2025 The LUCI Authors.
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

import { GpsFixed } from '@mui/icons-material';
import { Box, IconButton, Tooltip } from '@mui/material';

import {
  CategoryOption,
  MultiSelectCategoryChip,
} from '@/generic_libs/components/filter/multi_select_category_chip';
import { TestVerdict_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

import { useTestAggregationContext } from './context';

const STATUS_OPTIONS: CategoryOption[] = [
  { value: TestVerdict_Status[TestVerdict_Status.FAILED], label: 'Failed' },
  {
    value: TestVerdict_Status[TestVerdict_Status.EXECUTION_ERRORED],
    label: 'Execution Errored',
  },
  { value: TestVerdict_Status[TestVerdict_Status.FLAKY], label: 'Flaky' },
  { value: TestVerdict_Status[TestVerdict_Status.PASSED], label: 'Passed' },
  { value: TestVerdict_Status[TestVerdict_Status.SKIPPED], label: 'Skipped' },
  {
    value: TestVerdict_Status[TestVerdict_Status.PRECLUDED],
    label: 'Precluded',
  },
];

export function TestAggregationToolbar() {
  const { selectedStatuses, setSelectedStatuses, locateCurrentTest } =
    useTestAggregationContext();

  return (
    <Box
      sx={{
        p: 1,
        borderBottom: '1px solid #e0e0e0',
        display: 'flex',
        gap: 1,
        alignItems: 'center',
      }}
    >
      <Tooltip title="Locate current test">
        <IconButton size="small" onClick={locateCurrentTest}>
          <GpsFixed fontSize="small" />
        </IconButton>
      </Tooltip>
      <MultiSelectCategoryChip
        categoryName="Status"
        availableOptions={STATUS_OPTIONS}
        selectedItems={selectedStatuses}
        onSelectedItemsChange={setSelectedStatuses}
      />
    </Box>
  );
}
