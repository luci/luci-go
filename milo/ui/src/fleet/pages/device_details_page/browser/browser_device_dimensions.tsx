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

import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { StyledGrid } from '@/fleet/components/styled_data_grid';

import { BROWSER_COLUMN_OVERRIDES } from '../../device_list_page/browser/browser_columns';
import { BrowserDeviceDimensionsProps } from '../../device_list_page/browser/browser_devices_page';

export const BrowserDeviceDimensions = ({
  device,
}: BrowserDeviceDimensionsProps) => {
  if (device?.id === undefined) {
    return <></>;
  }

  const customRows = [{ id: 'id', value: device.id }];

  const customRowsId = customRows.map((r) => r.id);

  const labelRows = Object.keys(device.swarmingLabels)
    .filter((key) => !customRowsId.includes(key))
    .map((label) => {
      return {
        id: 'swarming.' + label,
        value: labelValuesToString(device!.swarmingLabels![label].values),
      };
    })
    .concat(
      Object.keys(device.ufsLabels)
        .filter((key) => !customRowsId.includes(key))
        .map((label) => {
          return {
            id: 'ufs.' + label,
            value: labelValuesToString(device!.ufsLabels![label].values),
          };
        }),
    )
    .sort((a, b) => a.id.localeCompare(b.id));

  const rows = [...customRows, ...labelRows];

  const columns: GridColDef[] = [
    { field: 'id', headerName: 'Key', flex: 1 },
    {
      field: 'value',
      headerName: 'Value',
      flex: 3,
      renderCell: (params: GridRenderCellParams) => {
        const override = BROWSER_COLUMN_OVERRIDES[params.id as string];

        if (override?.renderCell && params.id.toString() !== 'id') {
          return override.renderCell(params);
        }
        return <EllipsisTooltip>{params.value ?? ''}</EllipsisTooltip>;
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
        hideFooterPagination
      />
    )
  );
};
