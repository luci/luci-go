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

import {
  COLUMN_OVERRIDES,
  labelValuesToString,
} from '@/fleet/components/device_table/dimensions';
import { StyledGrid } from '@/fleet/components/styled_data_grid';
import { CellWithTooltip } from '@/fleet/components/table/cell_with_tooltip';
import { getDeviceStateString } from '@/fleet/utils/devices';
import {
  Device,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

interface DeviceDimensionsProps {
  device?: Device;
}

export const DeviceDimensions = (
  { device }: DeviceDimensionsProps,
  platform = Platform.CHROMEOS,
) => {
  if (device?.deviceSpec === undefined) {
    return <></>;
  }

  const labelRows = Object.keys(device.deviceSpec.labels).map((label) => {
    return {
      id: label,
      value: labelValuesToString(device!.deviceSpec!.labels[label].values),
    };
  });
  const rows = [
    { id: 'dut_id', value: device.dutId },
    { id: 'lease_state', value: getDeviceStateString(device) },
    ...labelRows,
  ];

  const columns: GridColDef[] = [
    { field: 'id', headerName: 'Key', flex: 1 },
    {
      field: 'value',
      headerName: 'Value',
      flex: 3,
      renderCell: (props: GridRenderCellParams) =>
        COLUMN_OVERRIDES[platform][props.id]?.renderCell?.(props) || (
          <CellWithTooltip {...props}></CellWithTooltip>
        ),
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
