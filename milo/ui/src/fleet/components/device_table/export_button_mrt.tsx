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

import SaveAltIcon from '@mui/icons-material/SaveAlt';
import { Button, Menu, Snackbar, Alert, AlertColor } from '@mui/material';
import { MRT_TableInstance, MRT_RowData } from 'material-react-table';
import { useState, SyntheticEvent, useMemo } from 'react';

import { chromeOSFriendlyNames } from '@/fleet/pages/device_list_page/chromeos/chromeos_fields';

import { CSVExportMenuItem } from './csv_export_menu_item';

const FILE_NAME = 'fleet_console_devices';

interface ExportButtonMrtProps<TData extends MRT_RowData> {
  table: MRT_TableInstance<TData>;
  selectedRowIds: string[];
}

interface NotificationState {
  open: boolean;
  message: string;
  severity: AlertColor;
}

export function ExportButton_MRT<TData extends MRT_RowData>({
  table,
  selectedRowIds,
}: ExportButtonMrtProps<TData>) {
  const columnsToExport = useMemo(
    () =>
      table
        .getVisibleLeafColumns()
        .filter(
          (column) =>
            column.id !== 'mrt-row-select' && column.id !== '__check__',
        )
        .map((column) => ({
          name: column.id,
          displayName:
            chromeOSFriendlyNames[column.id] ||
            (typeof column.columnDef.header === 'string'
              ? column.columnDef.header
              : column.id),
        })),
    [table],
  );
  const exportSelected = selectedRowIds.length > 0;

  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const handleClose = () => {
    setAnchorEl(null);
  };

  const [notification, setNotification] = useState<NotificationState>({
    open: false,
    message: '',
    severity: 'info',
  });

  const showNotification = (message: string, severity: AlertColor) => {
    setNotification({ open: true, message, severity });
  };

  const handleNotificationClose = (
    _: SyntheticEvent | Event,
    reason?: string,
  ) => {
    if (reason === 'clickaway') {
      return;
    }
    setNotification((prev) => ({ ...prev, open: false }));
  };

  return (
    <>
      <Button
        onClick={(event) => setAnchorEl(event.currentTarget)}
        size="small"
        startIcon={<SaveAltIcon />}
      >
        Export
      </Button>
      <Menu anchorEl={anchorEl} open={!!anchorEl} onClose={handleClose}>
        <CSVExportMenuItem
          displayText="Export all (CSV)"
          columnsToExport={columnsToExport}
          onExportComplete={handleClose}
          fileName={FILE_NAME}
          showNotification={showNotification}
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
              : table.getRowModel().rows.map((row) => row.id)
          }
          onExportComplete={handleClose}
          fileName={FILE_NAME}
          showNotification={showNotification}
        />
      </Menu>
      <Snackbar
        open={notification.open}
        autoHideDuration={3000}
        onClose={handleNotificationClose}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert
          onClose={handleNotificationClose}
          severity={notification.severity}
          variant="filled"
          sx={{ width: '100%' }}
        >
          {notification.message}
        </Alert>
      </Snackbar>
    </>
  );
}
