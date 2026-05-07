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

import Typography from '@mui/material/Typography';
import { MRT_ColumnDef } from 'material-react-table';
import React from 'react';

import { TaskResult } from '@/fleet/components/device_table/use_current_tasks';
import { FCHtmlTooltip } from '@/fleet/components/fc_html_tooltip';
import { CellWithTooltip } from '@/fleet/components/table';
import {
  renderChipCell,
  StateUnion,
} from '@/fleet/components/table/cell_with_chip';
import { renderCellWithLink } from '@/fleet/components/table/cell_with_link';
import { generateDutNameRedirectURL } from '@/fleet/config/device_config';
import { getSwarmingStateDocLinkForLabel } from '@/fleet/config/flops_doc_mapping';
import { generateChromeOsDeviceDetailsURL } from '@/fleet/constants/paths';
import { getStatusColor } from '@/fleet/pages/device_list_page/chromeos/dut_state';
import { FC_CellProps } from '@/fleet/types/table';
import {
  DEVICE_TASKS_SWARMING_HOST,
  generateBuildUrl,
} from '@/fleet/utils/builds';
import { getDeviceStateString } from '@/fleet/utils/devices';
import { getTaskURL } from '@/fleet/utils/swarming';
import {
  Device,
  DeviceState,
  DeviceType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export const chromeOSFriendlyNames: Record<string, string> = {
  id: 'ID',
  dut_id: 'Dut ID',
  type: 'Type',
  state: 'Lease state',
  host: 'Address',
  port: 'Port',
  current_task: 'Current Task',
  // TODO: Address underlying mapping issue to use friendly name.
  // User explicitly requested to keep 'dut_state' raw to avoid mapping issues.
  dut_state: 'dut_state',
  'label-servo_state': 'Servo State',
  bluetooth_state: 'Bluetooth State',
  'label-model': 'Model',
  'label-board': 'Board',
  dut_name: 'Dut Name',
  'label-associated_hostname': 'Associated Hostname',
};

// current_task is populated separated from ListDevices through direct calls to the Swarming
// API, requiring device data to be merged with task data.
export type ChromeOSDevice = Device & { current_task?: TaskResult };

export type ChromeOSColumnDef = MRT_ColumnDef<ChromeOSDevice> & {
  orderByField?: string;
  filterByField?: string;
};

export type ChromeOSColumnOverride = Omit<
  Partial<ChromeOSColumnDef>,
  'Cell'
> & {
  renderCell?: (props: FC_CellProps<ChromeOSDevice>) => React.ReactNode;
};

export const CHROMEOS_COLUMN_OVERRIDES: Record<string, ChromeOSColumnOverride> =
  {
    id: {
      header: 'ID',
      accessorFn: (device) => device.id,
      renderCell: renderCellWithLink<ChromeOSDevice>({
        linkGenerator: (value) => generateChromeOsDeviceDetailsURL(value),
        newTab: false,
      }),
      orderByField: 'id',
      filterByField: 'id',
    },
    dut_id: {
      header: 'Dut ID',
      accessorFn: (device) => device.dutId,
      orderByField: 'dut_id',
      filterByField: 'dut_id',
    },
    type: {
      header: 'Type',
      accessorFn: (device) => DeviceType[device.type],
      orderByField: 'type',
      filterByField: 'type',
    },
    state: {
      header: 'Lease state',
      accessorFn: (device) => getDeviceStateString(device),
      orderByField: 'state',
      filterByField: 'state',
      renderCell: (params: FC_CellProps<ChromeOSDevice>) => {
        const stateValue = String(params.cell.getValue() ?? '');

        if (stateValue === '') return null;

        const stateToTooltip: Record<string, string> = {
          [DeviceState.DEVICE_STATE_UNSPECIFIED]:
            'This device is not available for lease.',
        };

        const device = params.row.original;
        if (stateToTooltip[device.state] !== undefined) {
          return (
            <FCHtmlTooltip
              title={
                <Typography color="inherit" variant="body2">
                  {stateToTooltip[device.state]}
                </Typography>
              }
            >
              <span>{stateValue}</span>
            </FCHtmlTooltip>
          );
        } else {
          return (
            <CellWithTooltip
              column={params.column}
              value={stateValue}
            ></CellWithTooltip>
          );
        }
      },
    },
    host: {
      header: 'Address',
      accessorFn: (device) => device.address?.host || '',
      orderByField: 'host',
      filterByField: 'host',
    },
    port: {
      header: 'Port',
      accessorFn: (device) => device.address?.port?.toString() ?? '',
      orderByField: 'port',
      filterByField: 'port',
    },
    current_task: {
      header: 'Current Task',
      enableSorting: false,
      enableColumnFilter: false,
      accessorFn: (row) => row.current_task,
      renderCell: (props: FC_CellProps<ChromeOSDevice>) => {
        const val = props.cell.getValue<TaskResult>();
        if (val === 'loading') {
          return <></>;
        }

        if (
          val === undefined ||
          val === null ||
          (!val.taskName && !val.taskId)
        ) {
          return <CellWithTooltip column={props.column} value="idle" />;
        }
        return renderCurrentTaskCell(props);
      },
    },
    dut_state: {
      header: 'dut_state',
      accessorFn: (device) =>
        device.deviceSpec?.labels['dut_state']?.values?.[0]?.toUpperCase() ??
        '',
      renderCell: (params: FC_CellProps<ChromeOSDevice>) => {
        const stateValue = String(params.cell.getValue() ?? '');

        if (stateValue === '') return <></>;

        return renderChipCell<ChromeOSDevice>(
          getSwarmingStateDocLinkForLabel,
          getStatusColor,
          undefined,
          true,
          stateValue.toUpperCase() as StateUnion,
        )(params);
      },
    },
    'label-servo_state': {
      renderCell: renderCellWithLink<ChromeOSDevice>({
        linkGenerator: getSwarmingStateDocLinkForLabel,
      }),
    },
    bluetooth_state: {
      renderCell: renderCellWithLink<ChromeOSDevice>({
        linkGenerator: getSwarmingStateDocLinkForLabel,
      }),
    },
    'label-model': {
      renderCell: renderCellWithLink<ChromeOSDevice>({
        linkGenerator: (value) => `http://go/dlm-model/${value}`,
      }),
    },
    'label-board': {
      renderCell: renderCellWithLink<ChromeOSDevice>({
        linkGenerator: (value) => `http://go/dlm-board/${value}`,
      }),
    },
    dut_name: {
      renderCell: renderCellWithLink<ChromeOSDevice>({
        linkGenerator: generateDutNameRedirectURL,
      }),
    },
    'label-associated_hostname': {
      renderCell: renderCellWithLink<ChromeOSDevice>({
        linkGenerator: generateDutNameRedirectURL,
      }),
    },
    'label-primary_dut': {
      renderCell: renderCellWithLink<ChromeOSDevice>({
        linkGenerator: generateDutNameRedirectURL,
      }),
    },
    'label-managed_dut': {
      renderCell: renderCellWithLink<ChromeOSDevice>({
        linkGenerator: generateDutNameRedirectURL,
      }),
    },
    'label-servo_usb_state': {
      renderCell: renderCellWithLink<ChromeOSDevice>({
        linkGenerator: (value) => getSwarmingStateDocLinkForLabel(`${value}-2`),
      }),
    },
    bot_id: {
      renderCell: renderCellWithLink<ChromeOSDevice>({
        linkGenerator: (value) =>
          `https://${DEVICE_TASKS_SWARMING_HOST}/bot?id=${value}`,
      }),
    },
    realm: {
      header: 'Realm',
      accessorFn: (device) => device.realm || '',
      orderByField: 'realm',
      filterByField: 'realm',
    },
  };

const renderCurrentTaskCell = renderCellWithLink<ChromeOSDevice>({
  linkGenerator: (_task, device) => {
    const task = device.current_task;
    if (!task || task === 'loading') {
      return '';
    }
    const buildRegex =
      /^bb-(?<buildId>\d+)-(?<project>[^/]+)\/(?<bucket>[^/]+)\/(?<builder>[^/]+)$/;
    const match = task.taskName.match(buildRegex);
    if (match?.groups) {
      const { project, bucket, builder, buildId } = match.groups;
      return generateBuildUrl({
        project,
        bucket,
        builder,
        buildId: `b${buildId}`,
      });
    }
    return getTaskURL(task.taskId);
  },
  valueGetter: (device) => {
    const task = device.current_task;
    if (!task || task === 'loading') {
      return '';
    }
    return task.taskName ? `${task.taskName} (${task.taskId})` : task.taskId;
  },
});

export const EXTRA_COLUMN_IDS = [
  'id',
  'dut_id',
  'current_task',
  'realm',
] satisfies (keyof typeof CHROMEOS_COLUMN_OVERRIDES)[];
