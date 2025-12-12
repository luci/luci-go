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

import { DeviceTableGridColDef } from '@/fleet/components/device_table/device_table';
import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { CellWithTooltip } from '@/fleet/components/table';
import { renderCellWithLink } from '@/fleet/components/table/cell_with_link';
import { generateDutNameRedirectURL } from '@/fleet/config/device_config';
import { getSwarmingStateDocLinkForLabel } from '@/fleet/config/flops_doc_mapping';
import { generateChromeOsDeviceDetailsURL } from '@/fleet/constants/paths';
import { getDeviceStateString } from '@/fleet/utils/devices';
import { getTaskURL } from '@/fleet/utils/swarming';
import {
  Device,
  DeviceType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

// current_task is populated separated from ListDevices through direct calls to the Swarming
// API, requiring device data to be merged with task data.
type ChromeOSDevice = Device & { current_task?: string };

export const getColumns = (
  columnIds: string[],
): DeviceTableGridColDef<Device>[] => {
  return columnIds.map((id) => {
    return {
      field: id,
      headerName: id,
      orderByField: 'labels.' + id,
      editable: false,
      minWidth: 70,
      maxWidth: 700,
      sortable: true,
      valueGetter: (_, device) => {
        const labels = device.deviceSpec?.labels?.[id]?.values;
        if (!labels) return undefined;

        return labelValuesToString(labels);
      },
      flex: 1,
      renderCell: (param) => (
        <EllipsisTooltip>{param.value ?? ''}</EllipsisTooltip>
      ),
      ...(CHROMEOS_COLUMN_OVERRIDES[id] ?? {}),
    };
  });
};

/**
 * BASE_DIMENSIONS are dimensions associated with a device that are not labels,
 * which essentially defined how the UI renders non-label fields from the
 * `ListDevices` response.
 */
// export const BASE_DIMENSIONS: DimensionOverride = {

export const CHROMEOS_COLUMN_OVERRIDES: Record<
  string,
  Partial<DeviceTableGridColDef<Device>>
> = {
  id: {
    headerName: 'ID',
    valueGetter: (_, device) => device.id,
    renderCell: renderCellWithLink(
      (value) => generateChromeOsDeviceDetailsURL(value),
      false,
    ),
    orderByField: 'id',
  },
  // TODO(b/400711755): Rename this to Asset Tag
  dut_id: {
    headerName: 'Dut ID',
    valueGetter: (_, device) => device.dutId,
    orderByField: 'dut_id',
  },
  type: {
    headerName: 'Type',
    valueGetter: (_, device) => DeviceType[device.type],
    orderByField: 'type',
  },
  state: {
    headerName: 'Lease state',
    valueGetter: (_, device) => getDeviceStateString(device),
    orderByField: 'state',
  },
  host: {
    headerName: 'Address',
    valueGetter: (_, device) => device.address?.host || '',
    orderByField: 'host',
  },
  port: {
    headerName: 'Port',
    valueGetter: (_, device) => String(device.address?.port) || '',
    orderByField: 'port',
  },
  current_task: {
    headerName: 'Current Task',
    sortable: false,
    valueGetter: (_, row) => (row as ChromeOSDevice).current_task,
    renderCell: (props) => {
      if (props.value === undefined) {
        return <></>;
      }
      if (props.value === '') {
        return <CellWithTooltip {...props} value="idle"></CellWithTooltip>;
      }
      return renderCurrentTaskCell(props);
    },
  },
  dut_state: {
    valueGetter: (_, device) =>
      device.deviceSpec?.labels['dut_state']?.values?.[0]?.toUpperCase() ?? '',
    renderCell: renderCellWithLink(getSwarmingStateDocLinkForLabel),
  },
  'label-servo_state': {
    renderCell: renderCellWithLink(getSwarmingStateDocLinkForLabel),
  },
  bluetooth_state: {
    renderCell: renderCellWithLink(getSwarmingStateDocLinkForLabel),
  },
  'label-model': {
    renderCell: renderCellWithLink((value) => `http://go/dlm-model/${value}`),
  },
  'label-board': {
    renderCell: renderCellWithLink((value) => `http://go/dlm-board/${value}`),
  },
  dut_name: {
    renderCell: renderCellWithLink(generateDutNameRedirectURL),
  },
  'label-associated_hostname': {
    renderCell: renderCellWithLink(generateDutNameRedirectURL),
  },
  'label-primary_dut': {
    renderCell: renderCellWithLink(generateDutNameRedirectURL),
  },
  'label-managed_dut': {
    renderCell: renderCellWithLink(generateDutNameRedirectURL),
  },
  'label-servo_usb_state': {
    renderCell: renderCellWithLink((value) =>
      getSwarmingStateDocLinkForLabel(`${value}-2`),
    ),
  },
};

const renderCurrentTaskCell = renderCellWithLink((task_id) =>
  getTaskURL(task_id),
);
