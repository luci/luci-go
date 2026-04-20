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
  ListItemText,
  MenuItem,
  CircularProgress,
  AlertColor,
} from '@mui/material';

import { getErrorMessage } from '@/fleet/utils/errors';
import { exportAs } from '@/fleet/utils/export';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import { Column } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { useExportData } from './use_export_data';

export type CSVExportMenuItemProps = {
  displayText: string;
  columnsToExport: Column[];
  idsToExport?: string[];
  onExportComplete?: () => void;
  fileName: string;
  showNotification: (message: string, severity: AlertColor) => void;
};

export function CSVExportMenuItem({
  displayText,
  columnsToExport,
  idsToExport,
  onExportComplete,
  fileName,
  showNotification,
}: CSVExportMenuItemProps) {
  const { trackEvent } = useGoogleAnalytics();
  const { isFetching, refetch } = useExportData(columnsToExport, idsToExport);

  return (
    <MenuItem
      disabled={isFetching}
      onClick={async () => {
        trackEvent('export_csv', {
          componentName: 'export_csv_button',
          dutCount: idsToExport?.length,
        });
        const result = await refetch();

        if (result.isError) {
          showNotification(
            `An error occurred during CSV export: ${getErrorMessage(result.error, 'csv export')}`,
            'error',
          );
        } else {
          const csvData = result.data?.csvData;
          if (csvData !== undefined) {
            const blob = new Blob([csvData], {
              type: 'text/csv',
            });

            exportAs(blob, 'csv', fileName);
          }
          // Hide the export menu after the export success
          onExportComplete?.();
        }
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
              color: 'inherit',
            }}
          />
        )}
      </Box>
    </MenuItem>
  );
}
