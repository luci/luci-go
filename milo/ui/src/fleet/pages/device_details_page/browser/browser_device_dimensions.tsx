// Copyright 2024 The LUCI Authors.
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

import { GridColDef, GridRenderCellParams } from '@mui/x-data-grid';
import { useMemo } from 'react';

import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { StyledGrid } from '@/fleet/components/styled_data_grid';
import { BrowserDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import {
  BROWSER_COLUMN_OVERRIDES,
  getBrowserColumn,
  getBrowserColumnIds,
} from '../../device_list_page/browser/browser_columns';

export interface BrowserDeviceDimensionsProps {
  device?: BrowserDevice;
}

export const BrowserDeviceDimensions = ({
  device,
}: BrowserDeviceDimensionsProps) => {
  const rows = useMemo(() => {
    if (device?.id === undefined) {
      return [];
    }
    const columnsRecord = getBrowserColumnIds(undefined, [device]).map((id) =>
      getBrowserColumn(id),
    );

    return columnsRecord
      .map((col) => {
        return {
          key: col.headerName ?? col.field,
          value:
            col.valueGetter?.('' as never, device, col, {
              current: {} as never,
            }) ?? '',
          // This field is unique and will be used
          // to find custom renderCell functions.
          id: col.field,
        };
      })
      .sort((a, b) => {
        if (a.id === 'id') return -1;
        if (b.id === 'id') return 1;
        return a.id.localeCompare(b.id);
      });
  }, [device]);

  if (device?.id === undefined) {
    return <></>;
  }

  const columns: GridColDef[] = [
    { field: 'id' },
    { field: 'key', headerName: 'Key', flex: 1 },
    {
      field: 'value',
      headerName: 'Value',
      flex: 3,
      renderCell: (params: GridRenderCellParams) => {
        const override = BROWSER_COLUMN_OVERRIDES[params.id];

        if (override?.renderCell && params.id.toString() !== 'id') {
          // This imitates the behavior of the renderCell of the list page.
          return override.renderCell({ ...params, row: device });
        }
        return <EllipsisTooltip>{params.value}</EllipsisTooltip>;
      },
    },
  ];

  return (
    device && (
      <StyledGrid
        disableColumnMenu
        disableColumnFilter
        disableRowSelectionOnClick
        rows={rows}
        columns={columns}
        initialState={{
          columns: {
            columnVisibilityModel: {
              id: false,
            },
          },
        }}
        hideFooterPagination
      />
    )
  );
};
