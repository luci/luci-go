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

import { Box } from '@mui/material';
import {
  GridRowModel,
  GridToolbarContainer,
  GridToolbarExport,
} from '@mui/x-data-grid';

import { RunAutorepair } from '../actions/autorepair/run_autorepair';

import { ColumnsButton } from './columns_button';

export interface FleetToolbarProps {
  selectedRows: GridRowModel[];
  isLoadingColumns?: boolean;
}

/**
 * Custom data table toolbar styled for Fleet's expected usage.
 */
export function FleetToolbar({
  selectedRows = [],
  isLoadingColumns,
}: FleetToolbarProps) {
  const selectedDuts = selectedRows.map((row) => ({
    name: `${row.dut_name}`,
    state: row.dut_state,
  }));

  return (
    <GridToolbarContainer>
      {selectedRows.length ? (
        <RunAutorepair selectedDuts={selectedDuts} />
      ) : (
        <></>
      )}
      <GridToolbarExport printOptions={{ disableToolbarButton: true }} />
      <Box sx={{ flexGrow: 1 }} />
      <ColumnsButton isLoading={isLoadingColumns} />
    </GridToolbarContainer>
  );
}
