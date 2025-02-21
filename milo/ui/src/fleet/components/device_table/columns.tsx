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

import { getSwarmingStateDocLinkForLabel } from '@/fleet/constants/flops_doc_mapping';
import {
  Device,
  DeviceState,
  DeviceType,
} from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { Cell } from './Cell';

interface Dimension {
  id: string; // unique id used for sorting and filtering
  displayName?: string;
  getValue: (device: Device) => string;
  renderCell?: (props: GridRenderCellParams) => React.JSX.Element;
}

const getPathnameWithParams = () => {
  return window.location.href.toString().split(window.location.host)[1];
};

interface CellWithLinkProps {
  cellProps: GridRenderCellParams;
  linkTo: string;
  linkText: string;
  tooltipTitle?: string;
  target?: string;
}

const getCellWithLink = (props: CellWithLinkProps) => {
  return (
    <Cell
      {...props.cellProps}
      value={
        <Link
          to={props.linkTo}
          state={{
            navigatedFromLink: getPathnameWithParams(),
          }}
          target={props.target || '_self'}
        >
          {props.linkText}
        </Link>
      }
      tooltipTitle={props.tooltipTitle || props.linkText}
    />
  );
};

const getCellWithLinkToSwarmingDocs = (props: GridRenderCellParams) => {
  return getCellWithLink({
    cellProps: props,
    linkTo: getSwarmingStateDocLinkForLabel(props.value),
    linkText: props.value,
    tooltipTitle: props.value,
    target: '_blank',
  });
};

export const BASE_DIMENSIONS: Dimension[] = [
  {
    id: 'id',
    displayName: 'ID',
    getValue: (device: Device) => device.id,
    renderCell: (props) =>
      getCellWithLink({
        cellProps: props,
        linkTo: `/ui/fleet/labs/devices/${props.value}`,
        linkText: props.value,
        tooltipTitle: props.value,
      }),
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
    displayName: 'Lease state',
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

export const DIMENSIONS: Dimension[] = [
  {
    id: 'dut_state',
    getValue: (device: Device) =>
      device.deviceSpec?.labels['dut_state']?.values[0] || '',
    renderCell: getCellWithLinkToSwarmingDocs,
  },
  {
    id: 'label-servo_state',
    getValue: (device: Device) =>
      device.deviceSpec?.labels['label-servo_state']?.values[0] || '',
    renderCell: getCellWithLinkToSwarmingDocs,
  },
  {
    id: 'bluetooth_state',
    getValue: (device: Device) =>
      device.deviceSpec?.labels['bluetooth_state']?.values[0] || '',
    renderCell: getCellWithLinkToSwarmingDocs,
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
    headerName:
      [...BASE_DIMENSIONS, ...DIMENSIONS].find((dim) => dim.id === id)
        ?.displayName || id,
    editable: false,
    minWidth: 70,
    maxWidth: 700,
    flex: 1,
    renderCell: (props) =>
      [...BASE_DIMENSIONS, ...DIMENSIONS]
        .find((dim) => dim.id === id)
        ?.renderCell?.(props) || <Cell {...props}></Cell>,
  }));

  return columns;
};
