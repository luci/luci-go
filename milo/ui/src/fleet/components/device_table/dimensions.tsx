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

import { GridRenderCellParams } from '@mui/x-data-grid';
import React from 'react';

import { generateDutNameRedirectURL } from '@/fleet/config/device_config';
import { getSwarmingStateDocLinkForLabel } from '@/fleet/config/flops_doc_mapping';
import { DIMENSION_SEPARATOR } from '@/fleet/constants/dimension_separator';
import { generateChromeOsDeviceDetailsURL } from '@/fleet/constants/paths';
import { getDeviceStateString } from '@/fleet/utils/devices';
import { getTaskURL } from '@/fleet/utils/swarming';
import {
  Device,
  DeviceType,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { renderCellWithLink } from '../table/cell_with_link';
import { CellWithTooltip } from '../table/cell_with_tooltip';

export const labelValuesToString = (labels: readonly string[]): string => {
  return labels
    .concat()
    .sort((a, b) => (a.length < b.length ? 1 : -1))
    .join(DIMENSION_SEPARATOR);
};

type DimensionOverride = Record<
  string, // unique id used for sorting and filtering
  {
    displayName?: string;
    sortable?: boolean;
    getValue?: (device: Device) => string;
    renderCell?: (props: GridRenderCellParams) => React.JSX.Element | undefined;
    orderByField?: string;
  }
>;

const renderCurrentTaskCell = renderCellWithLink((task_id) =>
  getTaskURL(task_id),
);

/**
 * BASE_DIMENSIONS are dimensions associated with a device that are not labels,
 * which essentially defined how the UI renders non-label fields from the
 * `ListDevices` response.
 */
export const BASE_DIMENSIONS: DimensionOverride = {
  id: {
    displayName: 'ID',
    getValue: (device: Device) => device.id,
    renderCell: renderCellWithLink(
      (value) => generateChromeOsDeviceDetailsURL(value),
      false,
    ),
    orderByField: 'id',
  },
  // TODO(b/400711755): Rename this to Asset Tag
  dut_id: {
    displayName: 'Dut ID',
    getValue: (device: Device) => device.dutId,
    orderByField: 'dut_id',
  },
  type: {
    displayName: 'Type',
    getValue: (device: Device) => DeviceType[device.type],
    orderByField: 'type',
  },
  state: {
    displayName: 'Lease state',
    getValue: getDeviceStateString,
    orderByField: 'state',
  },
  host: {
    displayName: 'Address',
    getValue: (device: Device) => device.address?.host || '',
    orderByField: 'host',
  },
  port: {
    displayName: 'Port',
    getValue: (device: Device) => String(device.address?.port) || '',
    orderByField: 'port',
  },
  current_task: {
    displayName: 'Current Task',
    sortable: false,
    renderCell: (props) => {
      if (props.value === '') {
        return <CellWithTooltip {...props} value="idle"></CellWithTooltip>;
      }
      return renderCurrentTaskCell(props);
    },
  },
};

/**
 * Customized ChromeOS config for applying custom overrides for different
 * dimensions. Used, for example, to add doc links to specific table cells.
 */
export const CROS_DIMENSION_OVERRIDES: DimensionOverride = {
  dut_state: {
    getValue: (device) =>
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

export const ANDROID_DIMENSION_OVERRIDES: DimensionOverride = {
  id: {
    renderCell: (props) => {
      const row = props.row;
      if (!(row.hostname && row.host_ip && row.id)) return undefined;

      return renderCellWithLink((_, { row }) => {
        if (row.fc_machine_type === 'device') {
          return `https://mobileharness-fe.corp.google.com/devicedetailview/${row.hostname}/${row.host_ip}/${row.id}`;
        } else {
          return `https://mobileharness-fe.corp.google.com/labdetailview/${row.hostname}/${row.host_ip}`;
        }
      })(props);
    },
  },
  host_group: {
    displayName: 'host_group',
    orderByField: 'host_group',
  },
  state: {
    displayName: 'state',
    orderByField: 'state',
  },
  hostname: {
    displayName: 'hostname',
    orderByField: 'hostname',
  },
  run_target: {
    displayName: 'run_target',
    orderByField: 'run_target',
  },
  lab_name: {
    displayName: 'lab_name',
    orderByField: 'lab_name',
  },
};

/**
 * Constant with all of the different configured overrides for columns. The
 * UI has default logic to handle all label data, but will apply special logic
 * in the case of certain commonly used labels (ie: labels containing
 * links to docs)
 */
export const COLUMN_OVERRIDES: Record<Platform, DimensionOverride> = {
  [Platform.UNSPECIFIED]: {},
  [Platform.CHROMEOS]: {
    ...BASE_DIMENSIONS,
    ...CROS_DIMENSION_OVERRIDES,
  },
  [Platform.ANDROID]: ANDROID_DIMENSION_OVERRIDES,
  [Platform.CHROMIUM]: {},
};
