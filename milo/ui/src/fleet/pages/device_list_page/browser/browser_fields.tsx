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

import _ from 'lodash';
import { DateTime } from 'luxon';
import { MRT_ColumnDef } from 'material-react-table';

import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { SmartRelativeTimestamp } from '@/fleet/components/smart_relative_timestamp';
import { CellWithTooltip } from '@/fleet/components/table';
import { BuganizerLink } from '@/fleet/components/table/buganizer_link';
import {
  renderChipCell,
  StateUnion,
} from '@/fleet/components/table/cell_with_chip';
import { renderCellWithLink } from '@/fleet/components/table/cell_with_link';
import {
  getBrowserSwarmingStateDocLinkForLabel,
  getSwarmingStateDocLinkForLabel,
} from '@/fleet/config/flops_doc_mapping';
import {
  BROWSER_SWARMING_SOURCE,
  BROWSER_UFS_SOURCE,
} from '@/fleet/constants/browser';
import { generateBrowserDeviceDetailsURL } from '@/fleet/constants/paths';
import { FC_CellProps } from '@/fleet/types/table';
import { getFilterQueryString } from '@/fleet/utils/search_param';
import { getTaskURL } from '@/fleet/utils/swarming';
import { BrowserDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getStatusColor } from '../chromeos/dut_state';

import { getDisplayName } from './alias';
import {
  getBrowserSwarmingStateColor,
  sortSwarmingStates,
} from './swarming_state';

export type BrowserColumnDef = MRT_ColumnDef<BrowserDevice> & {
  orderByField?: string;
  filterKey?: string;
};

export const CUSTOM_COLUMNS: Record<string, BrowserColumnDef> = {
  unhealthy_devices_ratio: {
    accessorKey: 'unhealthy_devices_ratio',
    header: 'unhealthy/total devices',
    enableSorting: false,
    orderByField: `unhealthy_devices_ratio`,
    accessorFn: (row) => {
      const total_devices = row.ufsLabels?.['total_devices']?.values?.[0];
      const unhealthy_devices =
        row.ufsLabels?.['unhealthy_devices']?.values?.[0];

      if (!total_devices || !unhealthy_devices) {
        return '';
      }

      return `${unhealthy_devices} / ${total_devices}`;
    },
    Cell: (params) => {
      const hostname = params.row.original.ufsLabels?.['hostname']?.values?.[0];
      if (!hostname) {
        return (
          <CellWithTooltip
            value={params.cell.getValue() as string}
            column={params.column}
          />
        );
      }
      return renderCellWithLink<BrowserDevice>({
        linkGenerator: () =>
          getFilterQueryString(
            { [`${BROWSER_UFS_SOURCE}.associated_hostname`]: [hostname] },
            undefined,
            undefined,
          ),
        newTab: false,
      })(params);
    },
  },
};

export const BROWSER_COLUMN_OVERRIDES: Record<
  string,
  Partial<BrowserColumnDef>
> = {
  id: {
    header: 'machine',
    size: 250,
    minSize: 150,
    accessorFn: (row) => row.id,
    Cell: (props: FC_CellProps<BrowserDevice>) => {
      const id = String(props.cell.getValue() ?? '');
      const hostname = props.row.original.ufsLabels?.['hostname']?.values?.[0];
      const names = hostname ? [id, hostname] : id;

      const CellWithLink = renderCellWithLink<BrowserDevice>({
        linkGenerator: (value) => generateBrowserDeviceDetailsURL(value),
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
          <BuganizerLink name={names} project="chromium" />
          <CellWithLink {...props} />
        </div>
      );
    },
  },
  [`${BROWSER_SWARMING_SOURCE}.current_task`]: {
    Cell: (params) => {
      const swarmingInstance =
        params.row.original?.ufsLabels?.['swarming_instance']?.values?.[0];
      const swarmingHost =
        swarmingInstance && `${swarmingInstance}.appspot.com`;

      if (
        (params.cell.getValue() as string) &&
        (params.cell.getValue() as string) !== 'idle' &&
        swarmingHost
      ) {
        return renderCellWithLink<BrowserDevice>({
          linkGenerator: (value) => getTaskURL(value, swarmingHost),
        })(params);
      } else {
        return (
          <CellWithTooltip
            value={params.cell.getValue() as string}
            column={params.column}
          />
        );
      }
    },
  },
  [`${BROWSER_SWARMING_SOURCE}.dut_state`]: {
    accessorFn: (device) =>
      device.swarmingLabels?.['dut_state']?.values?.[0]?.toUpperCase() ?? '',
    Cell: (params) => {
      const stateValue =
        params.row.original?.swarmingLabels?.[
          'dut_state'
        ]?.values?.[0]?.toUpperCase() ??
        (params.cell.getValue() as string) ??
        '';

      if (stateValue === '') return <></>;

      return renderChipCell<BrowserDevice>({
        getValueOrUrl: getSwarmingStateDocLinkForLabel,
        getColor: getStatusColor,
        overrideValue: stateValue.toUpperCase() as StateUnion,
        getTrackingEvent: (value) => ({
          eventName: 'state_doc_link_clicked',
          payload: { componentName: 'browser_dut_state', activeTab: value },
        }),
      })(params);
    },
  },
  [`${BROWSER_SWARMING_SOURCE}.device`]: {
    accessorFn: (device) => {
      const values = device.swarmingLabels?.['device']?.values;
      return values
        ? labelValuesToString(values.map((v) => getDisplayName(v, 'device')))
        : undefined;
    },
  },
  [`${BROWSER_SWARMING_SOURCE}.device_type`]: {
    accessorFn: (device) => {
      const values = device.swarmingLabels?.['device_type']?.values;
      return values
        ? labelValuesToString(
            values.map((v) => getDisplayName(v, 'device_type')),
          )
        : undefined;
    },
  },
  [`${BROWSER_SWARMING_SOURCE}.gpu`]: {
    accessorFn: (device) => {
      const values = device.swarmingLabels?.['gpu']?.values;
      return values
        ? labelValuesToString(values.map((v) => getDisplayName(v, 'gpu')))
        : undefined;
    },
  },
  [`${BROWSER_SWARMING_SOURCE}.os`]: {
    accessorFn: (device) => {
      const values = device.swarmingLabels?.['os']?.values;
      return values
        ? labelValuesToString(values.map((v) => getDisplayName(v, 'os')))
        : undefined;
    },
  },
  [`${BROWSER_SWARMING_SOURCE}.last_seen`]: {
    Cell: (params) => {
      const value = params.cell.getValue() as string;
      if (!value) {
        return null;
      }
      const dt = DateTime.fromISO(value);
      if (!dt.isValid) {
        return <>{value}</>;
      }
      return <SmartRelativeTimestamp date={dt} />;
    },
  },
  [`${BROWSER_SWARMING_SOURCE}.last_sync`]: {
    Cell: (params) => {
      const value = params.cell.getValue() as string;
      if (!value) {
        return null;
      }
      const dt = DateTime.fromISO(value);
      if (!dt.isValid) {
        return <>{value}</>;
      }
      return <SmartRelativeTimestamp date={dt} />;
    },
  },
  [`${BROWSER_SWARMING_SOURCE}.state`]: {
    Cell: (params) => {
      let stateValues = [
        ...(params.row.original?.swarmingLabels?.['state']?.values ?? []),
      ];

      if (stateValues.length === 0) return <></>;

      stateValues = sortSwarmingStates(stateValues.map((v) => v.toUpperCase()));

      return (
        <EllipsisTooltip tooltip={stateValues.join(', ')}>
          {stateValues.map((val, index) => {
            const ChipComponent = renderChipCell<BrowserDevice>({
              getValueOrUrl: getBrowserSwarmingStateDocLinkForLabel,
              getColor: getBrowserSwarmingStateColor,
              overrideValue: val as StateUnion,
              getTrackingEvent: (value) => ({
                eventName: 'state_doc_link_clicked',
                payload: {
                  componentName: 'browser_device_state',
                  activeTab: value,
                },
              }),
            });
            return (
              <span key={val + String(index)} style={{ marginRight: '4px' }}>
                {ChipComponent(params)}
              </span>
            );
          })}
        </EllipsisTooltip>
      );
    },
  },
  [`${BROWSER_UFS_SOURCE}.hostname`]: {
    header: 'host/bot_id',
    Cell: (params) => {
      const swarmingInstance =
        params.row.original?.ufsLabels?.['swarming_instance']?.values?.[0];
      const swarmingHost =
        swarmingInstance && `${swarmingInstance}.appspot.com`;

      if ((params.cell.getValue() as string) && swarmingHost) {
        return renderCellWithLink<BrowserDevice>({
          linkGenerator: (value) => `https://${swarmingHost}/bot?id=${value}`,
        })(params);
      } else {
        return (
          <CellWithTooltip
            value={params.cell.getValue() as string}
            column={params.column}
          />
        );
      }
    },
  },
  [`${BROWSER_UFS_SOURCE}.last_sync`]: {
    Cell: (params) => {
      const value = params.cell.getValue() as string;
      if (!value) {
        return null;
      }
      const dt = DateTime.fromISO(value);
      if (!dt.isValid) {
        return <>{value}</>;
      }
      return <SmartRelativeTimestamp date={dt} />;
    },
  },
  realm: {
    header: 'Realm',
    accessorFn: (row) => row.realm || '',
    orderByField: 'realm',
    filterKey: 'realm',
  },
};
