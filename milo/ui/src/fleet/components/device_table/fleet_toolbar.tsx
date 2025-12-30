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
  GridToolbarDensitySelector,
} from '@mui/x-data-grid';
import { useMemo } from 'react';

import { extractDutLabel, extractDutState } from '@/fleet/utils/devices';
import {
  Device,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { RunAutorepair } from '../actions/autorepair/run_autorepair';
import { CopyButton } from '../actions/copy/copy_button';
import { RequestRepair } from '../actions/request_repair/request_repair';
import { ColumnsButton } from '../columns/columns_button';

import { ExportButton } from './export_button';

export interface FleetToolbarProps {
  selectedRows: GridRowModel[];
  isLoadingColumns?: boolean;
  resetDefaultColumns?: () => void;
  temporaryColumns?: string[];
  addUserVisibleColumn?: (column: string) => void;
  platform: Platform;
}

/**
 * Custom data table toolbar styled for Fleet's expected usage.
 */
export function FleetToolbar({
  selectedRows = [],
  isLoadingColumns,
  resetDefaultColumns,
  temporaryColumns,
  addUserVisibleColumn,
  platform,
}: FleetToolbarProps) {
  const tools: Partial<Record<Platform, React.ReactNode>> = useMemo(
    () => ({
      [Platform.CHROMEOS]: (() => {
        if (selectedRows.length === 0) {
          return <ExportButton selectedRowIds={[]} />;
        }

        const selectedDuts = selectedRows.map((row) => ({
          name: `${row.id}`,
          dutId: `${row.dutId}`,
          state: extractDutState(row as Device),
          pool: extractDutLabel('label-pool', row as Device),
          board: extractDutLabel('label-board', row as Device),
          model: extractDutLabel('label-model', row as Device),
        }));

        return (
          <>
            <RunAutorepair selectedDuts={selectedDuts} />
            <CopyButton />
            <RequestRepair selectedDuts={selectedDuts} />
            <ExportButton
              selectedRowIds={selectedRows.map((row) => `${row.id}`)}
            />
          </>
        );
      })(),

      [Platform.ANDROID]: selectedRows.length > 0 && <CopyButton />,
    }),
    [selectedRows],
  );

  return (
    <GridToolbarContainer>
      {tools[platform] ?? null}
      <Box sx={{ flexGrow: 1 }} />
      <GridToolbarDensitySelector />
      <ColumnsButton
        isLoading={isLoadingColumns}
        resetDefaultColumns={resetDefaultColumns}
        temporaryColumns={temporaryColumns}
        addUserVisibleColumn={addUserVisibleColumn}
      />
    </GridToolbarContainer>
  );
}
