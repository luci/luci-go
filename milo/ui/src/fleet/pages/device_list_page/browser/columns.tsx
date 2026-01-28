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

import { GridRenderCellParams } from '@mui/x-data-grid';
import { DateTime } from 'luxon';

import { DeviceTableGridColDef } from '@/fleet/components/device_table/device_table';
import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { SmartRelativeTimestamp } from '@/fleet/components/smart_relative_timestamp';
import { CellWithTooltip } from '@/fleet/components/table';
import { renderChipCell } from '@/fleet/components/table/cell_with_chip';
import { renderCellWithLink } from '@/fleet/components/table/cell_with_link';
import { getSwarmingStateDocLinkForLabel } from '@/fleet/config/flops_doc_mapping';
import {
  BROWSER_SWARMING_SOURCE,
  BROWSER_UFS_SOURCE,
} from '@/fleet/constants/browser';
import { generateBrowserDeviceDetailsURL } from '@/fleet/constants/paths';
import { BrowserDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { getStatusColor } from '../chromeos/dut_state';

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

export const getColumn = (id: string): DeviceTableGridColDef<BrowserDevice> => {
  const { labelKey, source } = destructureColumnId(id);

  return {
    field: id,
    headerName: source ? `${source.replace('_labels', '')}.${labelKey}` : id,
    orderByField: id,
    filterByField: source ? `${source}."${labelKey}"` : id,
    editable: false,
    minWidth: 70,
    maxWidth: 700,
    sortable: true,
    valueGetter: (_, device) => {
      let values: readonly string[] | undefined = undefined;

      if (source === BROWSER_SWARMING_SOURCE) {
        values = device.swarmingLabels?.[labelKey]?.values;
      } else if (source === BROWSER_UFS_SOURCE) {
        values = device.ufsLabels?.[labelKey]?.values;
      }

      return values ? labelValuesToString(values) : undefined;
    },
    flex: 1,
    renderCell: (param) => (
      <EllipsisTooltip>{param.value ?? ''}</EllipsisTooltip>
    ),
    ...(BROWSER_COLUMN_OVERRIDES[id] ?? {}),
  };
};

export const BROWSER_COLUMN_OVERRIDES: Record<
  string,
  Partial<DeviceTableGridColDef<BrowserDevice>>
> = {
  id: {
    flex: 3,
    valueGetter: (_value, row) => row.id,
    renderCell: renderCellWithLink(
      (value) => generateBrowserDeviceDetailsURL(value),
      false,
    ),
  },
  [`${BROWSER_SWARMING_SOURCE}.dut_state`]: {
    valueGetter: (_, device) =>
      device.swarmingLabels['dut_state']?.values?.[0]?.toUpperCase() ?? '',
    renderCell: (params: GridRenderCellParams<BrowserDevice>) => {
      const stateValue =
        params.row?.swarmingLabels?.['dut_state']?.values?.[0]?.toUpperCase() ??
        params.value ??
        '';

      if (stateValue === '')
        return <EllipsisTooltip>{stateValue}</EllipsisTooltip>;

      return renderChipCell(
        getSwarmingStateDocLinkForLabel,
        getStatusColor,
      )({ ...params, value: stateValue.toUpperCase() });
    },
  },
  [`${BROWSER_SWARMING_SOURCE}.last_sync`]: {
    renderCell: (params) => {
      const value = params.value as string;
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
  [`${BROWSER_UFS_SOURCE}.hostname`]: {
    renderCell: (params: GridRenderCellParams<BrowserDevice>) => {
      const swarmingInstance =
        params.row?.ufsLabels?.['swarming_instance']?.values?.[0];
      const swarmingHost =
        swarmingInstance && `${swarmingInstance}.appspot.com`;

      if (params.value && swarmingHost) {
        return renderCellWithLink<BrowserDevice>((value) => {
          return `https://${swarmingHost}/bot?id=${value}`;
        })(params);
      } else {
        return <CellWithTooltip {...params}></CellWithTooltip>;
      }
    },
  },
  [`${BROWSER_UFS_SOURCE}.last_sync`]: {
    renderCell: (params) => {
      const value = params.value as string;
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
};
