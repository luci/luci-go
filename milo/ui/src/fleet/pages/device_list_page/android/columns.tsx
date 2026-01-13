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

import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { DateTime } from 'luxon';
import { Link } from 'react-router';

import { DeviceTableGridColDef } from '@/fleet/components/device_table/device_table';
import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { SmartRelativeTimestamp } from '@/fleet/components/smart_relative_timestamp';
import { renderCellWithLink } from '@/fleet/components/table/cell_with_link';
import {
  generateDeviceDetailsURL,
  ANDROID_PLATFORM,
} from '@/fleet/constants/paths';
import { AndroidDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { RunTargetColumnHeader } from './run_target_column_header';

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
      const internalLink = generateDeviceDetailsURL(ANDROID_PLATFORM, d.id);

      const type = d.omnilabSpec?.labels['fc_machine_type']?.values?.[0];
      const hostname = d.omnilabSpec?.labels['hostname']?.values?.[0];
      const hostIp = d.omnilabSpec?.labels['host_ip']?.values?.[0];

      let mhLink = '';
      if (type === 'host') {
        if (hostname && hostIp) {
          mhLink = `https://mobileharness-fe.corp.google.com/labdetailview/${hostname}/${hostIp}`;
        } else {
          const params = new URLSearchParams();
          params.append('filter', `"host_name":("${d.id}")`);
          mhLink = `https://mobileharness-fe.corp.google.com/lablistview?${params.toString()}`;
        }
      } else {
        if (hostname && hostIp) {
          mhLink = `https://mobileharness-fe.corp.google.com/devicedetailview/${hostname}/${hostIp}/${d.id}`;
        } else {
          const params = new URLSearchParams();
          params.append('filter', `"id":("${d.id}")`);
          mhLink = `https://mobileharness-fe.corp.google.com/devicelistview?${params.toString()}`;
        }
      }

      return (
        <div style={{ display: 'flex', alignItems: 'center' }}>
          <Link to={internalLink} style={{ marginRight: '8px' }}>
            {d.id}
          </Link>
          <a
            href={mhLink}
            target="_blank"
            rel="noopener noreferrer"
            style={{ display: 'flex', alignItems: 'center' }}
          >
            <OpenInNewIcon fontSize="small" />
          </a>
        </div>
      );
    },
  },
  host_group: {
    orderByField: 'host_group',
  },
  state: {
    orderByField: 'state',
    renderCell: (props) => {
      return renderCellWithLink<AndroidDevice>((_1, _2) => {
        return 'https://g3doc.corp.google.com/company/teams/chrome/ops/fleet/flops/android/labtechs.md?cl=head#device-terminology';
      })(props);
    },
  },
  hostname: {
    orderByField: 'hostname',
  },
  run_target: {
    valueGetter: (_, device) => device.runTarget,
    orderByField: 'run_target',
    minWidth: 100,
    renderHeader: () => <RunTargetColumnHeader />,
  },
  lab_name: {
    orderByField: 'lab_name',
  },
  fc_machine_type: {
    orderByField: 'fc_machine_type',
  },
  fc_is_offline: {
    orderByField: 'fc_is_offline',
  },
  ['label-run_target']: {
    orderByField: 'labels.run_target',
    valueGetter: (_, device) => {
      const labels = device.omnilabSpec?.labels?.run_target?.values;
      if (!labels) return undefined;

      return labelValuesToString(labels);
    },
  },
  ['label-id']: {
    orderByField: 'labels.id',
    valueGetter: (_, device) => {
      const labels = device.omnilabSpec?.labels?.id?.values;
      if (!labels) return undefined;

      return labelValuesToString(labels);
    },
  },
  ['label-hostname']: {
    orderByField: 'labels.hostname',
    valueGetter: (_, device) => {
      const labels = device.omnilabSpec?.labels?.hostname?.values;
      if (!labels) return undefined;

      return labelValuesToString(labels);
    },
  },
  'ufs.last_sync': {
    orderByField: 'labels.ufs.last_sync',
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
  'mh.last_sync': {
    orderByField: 'labels.mh.last_sync',
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
