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
import { Link } from 'react-router-dom';

import {
  Device,
  DeviceState,
  DeviceType,
} from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { Cell } from './Cell';

interface Dimension {
  id: string; // unique id used for sorting and filtering
  displayName: string;
  getValue: (device: Device) => string;
  renderCell?: (props: GridRenderCellParams) => React.JSX.Element;
}

// this is loosely defining default columns based on b/391621656
// technically we should avoid hardcoding labels anywhere, as we should treat them as a black box,
// but we should be safe in this case
export const DEFAULT_COLUMNS: string[] = [
  'id',
  'dut_id',
  'state',
  'board-name',
  'dut_name',
  'dut_state',
  'label-board',
  'label-model',
  'label-phase',
  'label-pool',
  'label-servo_component',
  'label-servo_state',
  'label-servo_usb_state',
];

export const BASE_DIMENSIONS: Dimension[] = [
  {
    id: 'id',
    displayName: 'ID',
    getValue: (device: Device) => device.id,
    renderCell: (props) => (
      <Cell
        {...props}
        value={
          <Link to={`/ui/fleet/labs/devices/${props.value}`}>
            {props.value}
          </Link>
        }
        tooltipTitle={props.value}
      />
    ),
  },
  {
    id: 'dut_id',
    displayName: 'Dut ID',
    getValue: (device: Device) => device.dutId,
  },
  {
    id: 'type',
    displayName: 'Type',
    getValue: (device: Device) => DeviceType[device.type],
  },
  {
    id: 'state',
    displayName: 'State',
    getValue: (device: Device) => DeviceState[device.state],
  },
  {
    id: 'address.host',
    displayName: 'Address',
    getValue: (device: Device) => device.address?.host || '',
  },
  {
    id: 'address.port',
    displayName: 'Port',
    getValue: (device: Device) => String(device.address?.port) || '',
  },
] as const;

export const getColumns = (columnIds: string[]): GridColDef[] => {
  // order columns as in BASE_DIMENSIONS and put labels at the end
  const columnsOrdered = columnIds.sort((a, b) => {
    const aindex = BASE_DIMENSIONS.findIndex((dim) => dim.id === a);
    const bindex = BASE_DIMENSIONS.findIndex((dim) => dim.id === b);
    if (aindex === bindex) return 0;
    if (aindex < 0) return 1; // a is a label
    if (bindex < 0) return -1; // b is a label
    return aindex < bindex ? -1 : 1;
  });

  const columns: GridColDef[] = columnsOrdered.map((id) => ({
    field: id,
    headerName: BASE_DIMENSIONS.find((dim) => dim.id === id)?.displayName || id,
    editable: false,
    minWidth: 70,
    maxWidth: 700,
    flex: 1,
    renderCell: (props) =>
      BASE_DIMENSIONS.find((dim) => dim.id === id)?.renderCell?.(props) || (
        <Cell {...props}></Cell>
      ),
  }));

  return columns;
};
