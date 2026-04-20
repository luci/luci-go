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

import Typography from '@mui/material/Typography';
import { MRT_ColumnDef } from 'material-react-table';

import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
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
import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
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
export type ChromeOSDevice = Device & { current_task?: string };

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

export const getChromeOSColumns = (
  columnIds: string[],
): ChromeOSColumnDef[] => {
  const topLevelProtoFields = [
    'id',
    'dut_id',
    'type',
    'state',
    'host',
    'port',
    'realm',
  ];

  return columnIds.map((id) => {
    const isTopLevelProtoField = topLevelProtoFields.includes(id);

    const override = CHROMEOS_COLUMN_OVERRIDES[id] ?? {};

    return {
      accessorKey: id,
      header: id,
      orderByField: 'labels.' + id,
      filterByField: isTopLevelProtoField ? id : `labels."${id}"`,
      enableEditing: false,
      minSize: 70,
      maxSize: 700,
      enableSorting: true,
      accessorFn: (device) => {
        const labels = device.deviceSpec?.labels?.[id]?.values;
        if (!labels) return undefined;

        return labelValuesToString(labels);
      },
      ...(override.renderCell
        ? {
            Cell: (props: FC_CellProps<ChromeOSDevice>) =>
              override.renderCell!(props),
          }
        : {
            Cell: ({ cell }: FC_CellProps<ChromeOSDevice>) => (
              <EllipsisTooltip>{String(cell.getValue() ?? '')}</EllipsisTooltip>
            ),
          }),
      ...(() => {
        const { renderCell: _, ...rest } = override;
        return rest;
      })(),
    };
  });
};

export const CHROMEOS_COLUMN_OVERRIDES: Record<string, ChromeOSColumnOverride> =
  {
    id: {
      header: 'ID',
      accessorFn: (device) => device.id,
      renderCell: renderCellWithLink<ChromeOSDevice>(
        (value) => generateChromeOsDeviceDetailsURL(value),
        false,
      ),
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

        if (stateValue === '') return <></>;

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
      accessorFn: (device) => String(device.address?.port) || '',
      orderByField: 'port',
      filterByField: 'port',
    },
    current_task: {
      header: 'Current Task',
      enableSorting: false,
      enableColumnFilter: false,
      accessorFn: (row) => row.current_task,
      renderCell: (props: FC_CellProps<ChromeOSDevice>) => {
        const val = props.cell.getValue();
        if (val === undefined || val === null) {
          return <></>;
        }
        const value = String(val);
        if (value === '') {
          return (
            <CellWithTooltip
              column={props.column}
              value="idle"
            ></CellWithTooltip>
          );
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
      renderCell: renderCellWithLink<ChromeOSDevice>(
        getSwarmingStateDocLinkForLabel,
      ),
    },
    bluetooth_state: {
      renderCell: renderCellWithLink<ChromeOSDevice>(
        getSwarmingStateDocLinkForLabel,
      ),
    },
    'label-model': {
      renderCell: renderCellWithLink<ChromeOSDevice>(
        (value) => `http://go/dlm-model/${value}`,
      ),
    },
    'label-board': {
      renderCell: renderCellWithLink<ChromeOSDevice>(
        (value) => `http://go/dlm-board/${value}`,
      ),
    },
    dut_name: {
      renderCell: renderCellWithLink<ChromeOSDevice>(
        generateDutNameRedirectURL,
      ),
    },
    'label-associated_hostname': {
      renderCell: renderCellWithLink<ChromeOSDevice>(
        generateDutNameRedirectURL,
      ),
    },
    'label-primary_dut': {
      renderCell: renderCellWithLink<ChromeOSDevice>(
        generateDutNameRedirectURL,
      ),
    },
    'label-managed_dut': {
      renderCell: renderCellWithLink<ChromeOSDevice>(
        generateDutNameRedirectURL,
      ),
    },
    'label-servo_usb_state': {
      renderCell: renderCellWithLink<ChromeOSDevice>((value) =>
        getSwarmingStateDocLinkForLabel(`${value}-2`),
      ),
    },
    bot_id: {
      renderCell: renderCellWithLink<ChromeOSDevice>(
        (value) => `https://${DEVICE_TASKS_SWARMING_HOST}/bot?id=${value}`,
      ),
    },
    realm: {
      header: 'Realm',
      accessorFn: (device) =>
        (device as Device & { realm?: string }).realm || '',
      orderByField: 'realm',
      filterByField: 'realm',
    },
  };

const renderCurrentTaskCell = renderCellWithLink<ChromeOSDevice>((task_id) =>
  getTaskURL(task_id),
);

export const EXTRA_COLUMN_IDS = [
  'id',
  'dut_id',
  'current_task',
  'realm',
] satisfies (keyof typeof CHROMEOS_COLUMN_OVERRIDES)[];
