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

import { DeviceTableGridColDef } from '@/fleet/components/device_table/device_table';
import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { renderChipCell } from '@/fleet/components/table/cell_with_chip';
import { renderCellWithLink } from '@/fleet/components/table/cell_with_link';
import { getSwarmingStateDocLinkForLabel } from '@/fleet/config/flops_doc_mapping';
import { BROWSER_SWARMING_SOURCE } from '@/fleet/constants/browser';
import { generateBrowserDeviceDetailsURL } from '@/fleet/constants/paths';
import { BrowserDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { getStatusColor } from '../chromeos/dut_state';

export const getColumn = (id: string): DeviceTableGridColDef<BrowserDevice> => {
  const firstDotIdx = id.indexOf('.');
  const source =
    firstDotIdx === -1
      ? undefined
      : id.slice(0, firstDotIdx).replace('_labels', '');
  const label = firstDotIdx === -1 ? id : id.slice(firstDotIdx + 2, -1); // Trims of the quotes

  return {
    field: id,
    headerName: source ? `${source}.${label}` : label,
    orderByField: id,
    editable: false,
    minWidth: 70,
    maxWidth: 700,
    sortable: true,
    valueGetter: (_value, device) => {
      if (!source) return undefined;
      const labels =
        source === BROWSER_SWARMING_SOURCE
          ? device.swarmingLabels?.[label]?.values
          : device.ufsLabels?.[label]?.values;
      if (!labels) return undefined;

      return labelValuesToString(labels);
    },
    flex: 1,
    renderCell: (param) => (
      <EllipsisTooltip>{param.value ?? ''}</EllipsisTooltip>
    ),
    ...(BROWSER_COLUMN_OVERRIDES[source ? `${source}.${label}` : label] ?? {}),
  };
};

export const BROWSER_COLUMN_OVERRIDES: Record<
  string,
  Partial<DeviceTableGridColDef<BrowserDevice>>
> = {
  id: {
    flex: 3,
    orderByField: 'id',
    valueGetter: (_value, row) => row.id,
    renderCell: renderCellWithLink(
      (value) => generateBrowserDeviceDetailsURL(value),
      false,
    ),
  },
  'swarming.dut_state': {
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
};
