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
import { getFilterQueryString } from '@/fleet/utils/search_param';
import { getTaskURL } from '@/fleet/utils/swarming';
import {
  BrowserDevice,
  GetBrowserDeviceDimensionsResponse,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getStatusColor } from '../chromeos/dut_state';

import {
  getBrowserSwarmingStateColor,
  sortSwarmingStates,
} from './swarming_state';

const destructureColumnId = (id: string) => {
  let source: string | undefined = undefined;

  if (id.startsWith(BROWSER_SWARMING_SOURCE)) {
    source = BROWSER_SWARMING_SOURCE;
  } else if (id.startsWith(BROWSER_UFS_SOURCE)) {
    source = BROWSER_UFS_SOURCE;
  }

  const labelKey = source ? id.replace(`${source}.`, '') : id;

  return {
    labelKey,
    source,
  };
};

const getColumnHeader = (labelKey: string, source?: string) => {
  if (source === BROWSER_SWARMING_SOURCE) {
    return `sw.${labelKey}`;
  } else if (source === BROWSER_UFS_SOURCE) {
    return `ufs.${labelKey}`;
  } else {
    return labelKey;
  }
};

export const getBrowserColumnIds = (
  dimensions?: GetBrowserDeviceDimensionsResponse,
  extraColumns: string[] = [],
): string[] => {
  const ids: string[] = [];
  if (dimensions) {
    ids.push(
      ...Object.keys(dimensions.baseDimensions)
        .concat(
          ...Object.keys(dimensions.swarmingLabels).map(
            (l) => `${BROWSER_SWARMING_SOURCE}.${l}`,
          ),
        )
        .concat(
          ...Object.keys(dimensions.ufsLabels).map(
            (l) => `${BROWSER_UFS_SOURCE}.${l}`,
          ),
        ),
    );
  }

  ids.push(...Object.keys(CUSTOM_COLUMNS));
  ids.push('realm');
  ids.push(...extraColumns);
  return _.uniq(ids);
};

export type BrowserColumnDef = MRT_ColumnDef<BrowserDevice> & {
  orderByField?: string;
  filterByField?: string;
};

export const getBrowserColumn = (id: string): BrowserColumnDef => {
  const customColumn = CUSTOM_COLUMNS[id];
  if (customColumn) {
    return customColumn;
  }

  const { labelKey, source } = destructureColumnId(id);

  return {
    accessorKey: id,
    header: getColumnHeader(labelKey, source),
    orderByField: id,
    filterByField: source ? `${source}."${labelKey}"` : id,
    enableEditing: false,
    minSize: 70,
    maxSize: 700,
    enableSorting: true,
    accessorFn: (device) => {
      let values: readonly string[] | undefined = undefined;

      if (source === BROWSER_SWARMING_SOURCE) {
        values = device.swarmingLabels?.[labelKey]?.values;
      } else if (source === BROWSER_UFS_SOURCE) {
        values = device.ufsLabels?.[labelKey]?.values;
      }

      return values ? labelValuesToString(values) : undefined;
    },
    Cell: (param) => (
      <EllipsisTooltip>
        {(param.renderedCellValue as React.ReactNode) ?? ''}
      </EllipsisTooltip>
    ),
    ...(BROWSER_COLUMN_OVERRIDES[id] ?? {}),
  };
};

const CUSTOM_COLUMNS: Record<string, BrowserColumnDef> = {
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
      return renderCellWithLink<BrowserDevice>(
        () =>
          getFilterQueryString(
            { [`${BROWSER_UFS_SOURCE}.associated_hostname`]: [hostname] },
            undefined,
            undefined,
          ),
        false,
      )(params);
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
    Cell: renderCellWithLink<BrowserDevice>(
      (value) => generateBrowserDeviceDetailsURL(value),
      false,
    ),
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
        return renderCellWithLink<BrowserDevice>((value) => {
          return getTaskURL(value, swarmingHost);
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

      return renderChipCell<BrowserDevice>(
        getSwarmingStateDocLinkForLabel,
        getStatusColor,
        undefined,
        true,
        stateValue.toUpperCase() as StateUnion,
      )(params);
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
            const ChipComponent = renderChipCell<BrowserDevice>(
              getBrowserSwarmingStateDocLinkForLabel,
              getBrowserSwarmingStateColor,
              undefined,
              true,
              val as StateUnion,
            );
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
        return renderCellWithLink<BrowserDevice>((value) => {
          return `https://${swarmingHost}/bot?id=${value}`;
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
    accessorFn: (row) =>
      (row as BrowserDevice & { realm?: string }).realm || '',
    orderByField: 'realm',
    filterByField: 'realm',
  },
};
