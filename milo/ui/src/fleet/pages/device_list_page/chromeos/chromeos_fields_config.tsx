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

import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { FCHtmlTooltip } from '@/fleet/components/fc_html_tooltip';
import { CellWithTooltip } from '@/fleet/components/table';
import { BuganizerLink } from '@/fleet/components/table/buganizer_link';
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
  DeviceState,
  DeviceType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { ChromeOSDevice, FieldDefinition } from './chromeos_types';
import { TaskResult } from './use_chromeos_current_tasks';

/**
 * Fetches the values of a label.
 * Prefer adding labels to CHROMEOS_FIELD_DEFINITIONS to maintain type safety and avoid hardcoded strings.
 */
const getLabelValuesInternal = (
  device: ChromeOSDevice,
  id: string,
): readonly string[] => {
  return device.deviceSpec?.labels?.[id]?.values ?? [];
};

export const getLabelValues = (
  device: ChromeOSDevice,
  id: KnownChromeOSColumnId | (string & {}),
): readonly string[] => {
  return getLabelValuesInternal(device, id);
};

const getLabelValueInternal = (device: ChromeOSDevice, id: string): string => {
  const labels = getLabelValuesInternal(device, id);
  if (labels.length === 0) return '';
  return labelValuesToString(labels as string[]);
};

export const getLabelValue = (
  device: ChromeOSDevice,
  id: KnownChromeOSColumnId | (string & {}),
): string => {
  return getLabelValueInternal(device, id);
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

/**
 * Centralized definition for all known fields (dimensions and labels) for ChromeOS devices.
 * If you need to use a label or dimension in the UI (e.g., for custom rendering, filtering, or sorting),
 * add it here to ensure type safety and avoid hardcoded strings.
 */
export const CHROMEOS_FIELD_DEFINITIONS = {
  id: {
    type: 'base',
    header: 'ID',
    accessorFn: (device) => device.id,
    orderByField: 'id',
    filterKey: 'id',
    renderCell: (props: FC_CellProps<ChromeOSDevice>) => {
      const id = String(props.cell.getValue() ?? '');
      const dutId = props.row.original.dutId;
      const names = dutId ? [id, dutId] : id;

      const CellWithLink = renderCellWithLink<ChromeOSDevice>({
        linkGenerator: (value) => generateChromeOsDeviceDetailsURL(value),
        newTab: false,
      });

      return (
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '4px',
            maxWidth: '100%',
            textOverflow: 'ellipsis',
          }}
        >
          <BuganizerLink name={names} project="chromeos" />
          <CellWithLink {...props} />
        </div>
      );
    },
  },
  dut_id: {
    type: 'base',
    header: 'Dut ID',
    accessorFn: (device) => device.dutId,
    orderByField: 'dut_id',
    filterKey: 'dut_id',
  },
  type: {
    type: 'base',
    header: 'Type',
    accessorFn: (device) => DeviceType[device.type],
    orderByField: 'type',
    filterKey: 'type',
  },
  state: {
    type: 'base',
    header: 'Lease state',
    accessorFn: (device) => getDeviceStateString(device),
    orderByField: 'state',
    filterKey: 'state',
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
    type: 'base',
    header: 'Address',
    accessorFn: (device) => device.address?.host || '',
    orderByField: 'host',
    filterKey: 'host',
  },
  port: {
    type: 'base',
    header: 'Port',
    accessorFn: (device) => device.address?.port?.toString() ?? '',
    orderByField: 'port',
    filterKey: 'port',
  },
  current_task: {
    type: 'special',
    header: 'Current Task',
    accessorFn: (row) => row.current_task,
    renderCell: (props: FC_CellProps<ChromeOSDevice>) => {
      const val = props.cell.getValue<TaskResult>();
      if (val === 'loading') {
        return <></>;
      }

      if (val === undefined || val === null || (!val.taskName && !val.taskId)) {
        return <CellWithTooltip column={props.column} value="idle" />;
      }
      return renderCurrentTaskCell(props);
    },
  },
  dut_state: {
    type: 'label',
    header: 'dut_state',
    accessorFn: (device) =>
      getLabelValueInternal(device, 'dut_state').toUpperCase(),
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
    type: 'label',
    header: 'Servo State',
    renderCell: renderCellWithLink<ChromeOSDevice>({
      linkGenerator: getSwarmingStateDocLinkForLabel,
    }),
  },
  bluetooth_state: {
    type: 'label',
    header: 'Bluetooth State',
    renderCell: renderCellWithLink<ChromeOSDevice>({
      linkGenerator: getSwarmingStateDocLinkForLabel,
    }),
  },
  'label-pool': {
    type: 'label',
  },
  'label-model': {
    type: 'label',
    header: 'Model',
    renderCell: renderCellWithLink<ChromeOSDevice>({
      linkGenerator: (value) => `http://go/dlm-model/${value}`,
    }),
  },
  'label-board': {
    type: 'label',
    header: 'Board',
    renderCell: renderCellWithLink<ChromeOSDevice>({
      linkGenerator: (value) => `http://go/dlm-board/${value}`,
    }),
  },
  dut_name: {
    type: 'label',
    header: 'Dut Name',
    renderCell: renderCellWithLink<ChromeOSDevice>({
      linkGenerator: generateDutNameRedirectURL,
    }),
  },
  'label-associated_hostname': {
    type: 'label',
    header: 'Associated Hostname',
    renderCell: renderCellWithLink<ChromeOSDevice>({
      linkGenerator: generateDutNameRedirectURL,
    }),
  },
  'label-primary_dut': {
    type: 'label',
    renderCell: renderCellWithLink<ChromeOSDevice>({
      linkGenerator: generateDutNameRedirectURL,
    }),
  },
  'label-managed_dut': {
    type: 'label',
    renderCell: renderCellWithLink<ChromeOSDevice>({
      linkGenerator: generateDutNameRedirectURL,
    }),
  },
  'label-servo_usb_state': {
    type: 'label',
    renderCell: renderCellWithLink<ChromeOSDevice>({
      linkGenerator: (value) => getSwarmingStateDocLinkForLabel(`${value}-2`),
    }),
  },
  bot_id: {
    type: 'label',
    renderCell: renderCellWithLink<ChromeOSDevice>({
      linkGenerator: (value) =>
        `https://${DEVICE_TASKS_SWARMING_HOST}/bot?id=${value}`,
    }),
  },
  ufs_namespace: {
    type: 'label',
  },
  realm: {
    type: 'base',
    accessorFn: (device) => device.realm || '',
    orderByField: 'realm',
    filterKey: 'realm',
  },
} satisfies Record<string, FieldDefinition>;

export type KnownChromeOSColumnId = keyof typeof CHROMEOS_FIELD_DEFINITIONS;
