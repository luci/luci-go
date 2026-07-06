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

import SaveAltIcon from '@mui/icons-material/SaveAlt';
import {
  Alert,
  AlertColor,
  Box,
  Button,
  CircularProgress,
  ListItemText,
  Menu,
  MenuItem,
  Snackbar,
} from '@mui/material';
import { MRT_RowData, MRT_TableInstance } from 'material-react-table';
import { useMemo, useState, SyntheticEvent } from 'react';

import { useFleetAnalytics } from '@/fleet/hooks/use_fleet_analytics';
import { getErrorMessage } from '@/fleet/utils/errors';
import { exportAs } from '@/fleet/utils/export';
import { Column } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export interface FleetCSVExportButtonProps<TData extends MRT_RowData> {
  table: MRT_TableInstance<TData>;
  filter: string;
  fileName: string;
  onExport: (
    columns: Column[],
    filter: string,
    ids?: string[],
  ) => Promise<{ csvData?: string }>;
}

interface NotificationState {
  open: boolean;
  message: string;
  severity: AlertColor;
}

export function FleetCSVExportButton<TData extends MRT_RowData>({
  table,
  filter,
  fileName,
  onExport,
}: FleetCSVExportButtonProps<TData>) {
  const { trackEvent } = useFleetAnalytics();
  const selectedRowIds = Object.keys(table.getState().rowSelection);
  const exportSelected = selectedRowIds.length > 0;

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
            typeof column.columnDef.header === 'string'
              ? column.columnDef.header
              : column.id,
        })),
    [table],
  );

  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const handleClose = () => setAnchorEl(null);

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
    if (reason === 'clickaway') return;
    setNotification((prev) => ({ ...prev, open: false }));
  };

  const [isExportingAll, setIsExportingAll] = useState(false);
  const [isExportingPage, setIsExportingPage] = useState(false);

  const executeExport = async (
    isAll: boolean,
    ids?: string[],
    filterStr: string = '',
  ) => {
    const setLoader = isAll ? setIsExportingAll : setIsExportingPage;
    setLoader(true);
    trackEvent('export_csv', {
      componentName: 'export_csv_button',
      dutCount: ids?.length,
    });

    try {
      const result = await onExport(columnsToExport, filterStr, ids);
      if (!result?.csvData) {
        showNotification('CSV export returned empty data', 'error');
      } else {
        const blob = new Blob([result.csvData], { type: 'text/csv' });
        exportAs(blob, 'csv', fileName);
        handleClose();
      }
    } catch (error) {
      showNotification(
        `An error occurred during CSV export: ${getErrorMessage(error, 'csv export')}`,
        'error',
      );
    } finally {
      setLoader(false);
    }
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
        <MenuItem
          disabled={isExportingAll || isExportingPage}
          onClick={() => executeExport(true, undefined, filter)}
        >
          <Box sx={{ position: 'relative' }}>
            <ListItemText primary="Export all (CSV)" />
            {isExportingAll && (
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
        <MenuItem
          disabled={isExportingAll || isExportingPage}
          onClick={() =>
            executeExport(
              false,
              exportSelected
                ? selectedRowIds
                : table.getRowModel().rows.map((row) => row.id),
              filter,
            )
          }
        >
          <Box sx={{ position: 'relative' }}>
            <ListItemText
              primary={
                exportSelected
                  ? 'Export selected (CSV)'
                  : 'Export current page (CSV)'
              }
            />
            {isExportingPage && (
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
