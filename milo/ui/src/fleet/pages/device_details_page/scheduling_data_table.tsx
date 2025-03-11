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

import { Cell } from '@/fleet/components/device_table/Cell';
import {
  COLUMN_OVERRIDES,
  labelValuesToString,
} from '@/fleet/components/device_table/dimensions';
import { StyledGrid } from '@/fleet/components/styled_data_grid';
import { Device } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

interface SchedulingDataProps {
  device?: Device;
}

export const SchedulingData = ({ device }: SchedulingDataProps) => {
  if (device?.deviceSpec === undefined) {
    return <></>;
  }

  const rows = Object.keys(device.deviceSpec.labels).map((label) => {
    return {
      id: label,
      value: labelValuesToString(device!.deviceSpec!.labels[label].values),
    };
  });

  const columns: GridColDef[] = [
    { field: 'id', headerName: 'Key', flex: 1 },
    {
      field: 'value',
      headerName: 'Value',
      flex: 3,
      renderCell: (props: GridRenderCellParams) =>
        COLUMN_OVERRIDES.find((dim) => dim.id === props.id)?.renderCell?.(
          props,
        ) || <Cell {...props}></Cell>,
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
