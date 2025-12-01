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
import { renderCellWithLink } from '@/fleet/components/table/cell_with_link';
import { AndroidDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

export const getColumns = (
  columnIds: string[],
): DeviceTableGridColDef<AndroidDevice>[] => {
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
        const labels = device.omnilabSpec?.labels?.[id]?.values;
        if (!labels) return undefined;

        return labelValuesToString(labels);
      },
      flex: 1,
      renderCell: (param) => (
        <EllipsisTooltip>{param.value ?? ''}</EllipsisTooltip>
      ),
      ...(ANDROID_COLUMN_OVERRIDES[id] ?? {}),
    };
  });
};

export const ANDROID_COLUMN_OVERRIDES: Record<
  string,
  Partial<DeviceTableGridColDef<AndroidDevice>>
> = {
  id: {
    flex: 3,
    orderByField: 'id',
    valueGetter: (_, device) => device.id,

    renderCell: (props) => {
      const d = props.row;

      const hostname = d.omnilabSpec?.labels['hostname']?.values?.[0];
      const hostIp = d.omnilabSpec?.labels['host_ip']?.values?.[0];

      if (!(hostname && hostIp && d.id)) return undefined;

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
    orderByField: 'host_group',
  },
  state: {
    orderByField: 'state',
  },
  hostname: {
    orderByField: 'hostname',
  },
  run_target: {
    valueGetter: (_, device) => device.runTarget,
    orderByField: 'run_target',
  },
  lab_name: {
    orderByField: 'lab_name',
  },
  fc_machine_type: {
    orderByField: 'fc_machine_type',
  },
};
