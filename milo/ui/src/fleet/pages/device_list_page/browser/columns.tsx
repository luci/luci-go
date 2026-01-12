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

import { DeviceTableGridColDef } from '@/fleet/components/device_table/device_table';
import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { BROWSER_SWARMING_SOURCE } from '@/fleet/constants/browser';
import { BrowserDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

export const getColumns = (
  columnIds: string[],
): DeviceTableGridColDef<BrowserDevice>[] => {
  return columnIds.map((id) => {
    const firstDotIdx = id.indexOf('.');
    const source = firstDotIdx === -1 ? undefined : id.slice(0, firstDotIdx);
    const label = firstDotIdx === -1 ? id : id.slice(firstDotIdx + 1);
    return {
      field: id,
      headerName: label,
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
      ...(BROWSER_COLUMN_OVERRIDES[id] ?? {}),
    };
  });
};

export const BROWSER_COLUMN_OVERRIDES: Record<
  string,
  Partial<DeviceTableGridColDef<BrowserDevice>>
> = {
  id: {
    flex: 3,
    orderByField: 'id',
    valueGetter: (_value, row) => row.id,
  },
};
