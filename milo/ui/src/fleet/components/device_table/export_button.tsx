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

import {
  Box,
  Button,
  CircularProgress,
  ListItemText,
  Menu,
  MenuItem,
} from '@mui/material';
import { useGridApiContext, GridSaveAltIcon } from '@mui/x-data-grid';
import { useNotifications } from '@toolpad/core/useNotifications';
import { useState } from 'react';

import { getErrorMessage } from '@/fleet/utils/errors';
import { exportAs } from '@/fleet/utils/export';
import { Column } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { useExportData } from './use_export_data';

const FILE_NAME = 'fleet_console_devices';

interface ExportButtonProps {
  selectedRowIds: string[];
}

type CSVExportMenuItemProps = {
  displayText: string;
  columnsToExport: Column[];
  idsToExport?: string[];
  onExportComplete?: () => void;
  fileName: string;
};

export function CSVExportMenuItem({
  displayText,
  columnsToExport,
  idsToExport,
  onExportComplete,
  fileName,
}: CSVExportMenuItemProps) {
  const notifications = useNotifications();

  const { isFetching, refetch } = useExportData(columnsToExport, idsToExport);

  return (
    <MenuItem
      disabled={isFetching}
      onClick={async () => {
        const result = await refetch();

        if (result.isError) {
          notifications.show(
            `An error occurred during CSV export: ${getErrorMessage(result.error, 'csv export')}`,
            {
              severity: 'error',
              autoHideDuration: 3000,
            },
          );
        } else {
          const csvData = result.data?.csvData;
          if (csvData !== undefined) {
            const blob = new Blob([csvData], {
              type: 'text/csv',
            });

            exportAs(blob, 'csv', fileName);
          }
        }

        // Hide the export menu after the export
        onExportComplete?.();
      }}
    >
      <Box>
        <ListItemText primary={displayText} />
        {isFetching && (
          <CircularProgress
            size={24}
            sx={{
              position: 'absolute',
              top: '50%',
              left: '50%',
              marginTop: '-12px',
              marginLeft: '-12px',
            }}
          />
        )}
      </Box>
    </MenuItem>
  );
}

export function ExportButton({ selectedRowIds }: ExportButtonProps) {
  const gridApi = useGridApiContext();
  const columnsToExport = gridApi.current
    .getVisibleColumns()
    .filter((column) => column.field !== '__check__')
    .map((column) => ({
      name: column.field,
      displayName: column.headerName ?? column.field,
    }));
  const exportSelected = selectedRowIds.length > 0;

  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const handleClose = () => {
    setAnchorEl(null);
  };

  return (
    <>
      <Button
        onClick={(event) => setAnchorEl(event.currentTarget)}
        size="small"
        startIcon={<GridSaveAltIcon />}
      >
        Export
      </Button>
      <Menu anchorEl={anchorEl} open={!!anchorEl} onClose={handleClose}>
        <CSVExportMenuItem
          displayText="Export all (CSV)"
          columnsToExport={columnsToExport}
          onExportComplete={handleClose}
          fileName={FILE_NAME}
        />
        <CSVExportMenuItem
          displayText={
            exportSelected
              ? 'Export selected (CSV)'
              : 'Export current page (CSV)'
          }
          columnsToExport={columnsToExport}
          // If nothing is selected, export current page
          idsToExport={
            exportSelected
              ? selectedRowIds
              : gridApi.current.getAllRowIds().map((id) => id as string)
          }
          onExportComplete={handleClose}
          fileName={FILE_NAME}
        />
      </Menu>
    </>
  );
}
