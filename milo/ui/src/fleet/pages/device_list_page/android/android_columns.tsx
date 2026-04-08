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

import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import PowerIcon from '@mui/icons-material/Power';
import PowerOffOutlinedIcon from '@mui/icons-material/PowerOffOutlined';
import { Tooltip } from '@mui/material';
import { DateTime } from 'luxon';
import { MRT_ColumnDef } from 'material-react-table';
import { Link } from 'react-router';

import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { SmartRelativeTimestamp } from '@/fleet/components/smart_relative_timestamp';
import {
  renderChipCell,
  StateUnion,
} from '@/fleet/components/table/cell_with_chip';
import {
  generateDeviceDetailsURL,
  ANDROID_PLATFORM,
} from '@/fleet/constants/paths';
import { FC_CellProps } from '@/fleet/types/table';
import { AndroidDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { getAndroidStatusColor } from './android_state';

export type AndroidColumnDef = MRT_ColumnDef<AndroidDevice> & {
  orderByField?: string;
  filterByField?: string;
};

export interface DeviceDisplayProps {
  value: unknown;
  device?: AndroidDevice;
}

export type AndroidColumnOverride = Omit<Partial<AndroidColumnDef>, 'Cell'> & {
  renderCell?: (props: DeviceDisplayProps) => React.ReactNode;
};

export const getAndroidColumns = (columnIds: string[]): AndroidColumnDef[] => {
  const topLevelProtoFields = [
    'id',
    'run_target',
    'realm',
    'fc_offline_since',
    'state',
    'hostname',
    'host_group',
  ];

  return columnIds.map((id) => {
    const isTopLevelProtoField = topLevelProtoFields.includes(id);

    const override = ANDROID_COLUMN_OVERRIDES[id] ?? {};
    const { renderCell, ...restOverride } = override;

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
        const labels = device.omnilabSpec?.labels?.[id]?.values;
        if (!labels) return undefined;

        return labelValuesToString(labels);
      },
      Cell: (param) => (
        <EllipsisTooltip>{param.renderedCellValue ?? ''}</EllipsisTooltip>
      ),
      ...restOverride,
      ...(renderCell
        ? {
            Cell: (params) =>
              renderCell({
                value: params.cell.getValue(),
                device: params.row.original,
              }),
          }
        : {}),
    };
  });
};

export const ANDROID_COLUMN_OVERRIDES: Record<string, AndroidColumnOverride> = {
  id: {
    size: 180,
    minSize: 120,
    orderByField: 'id',
    accessorFn: (device) => device.id,

    renderCell: ({ value, device }) => {
      const d = device;
      if (!d) return (value as React.ReactNode) ?? null;
      const internalLink = generateDeviceDetailsURL(ANDROID_PLATFORM, d.id);

      const type = d.omnilabSpec?.labels['fc_machine_type']?.values?.[0];
      const hostname = d.omnilabSpec?.labels['hostname']?.values?.[0];
      const hostIp = d.omnilabSpec?.labels['host_ip']?.values?.[0];

      let mhLink = '';
      if (type === 'host') {
        if (hostname && hostIp) {
          mhLink = `https://mobileharness-fe.corp.google.com/labdetailview/${hostname}/${hostIp}`;
        } else {
          const urlParams = new URLSearchParams();
          urlParams.append('filter', `"host_name":("${d.id}")`);
          mhLink = `https://mobileharness-fe.corp.google.com/lablistview?${urlParams.toString()}`;
        }
      } else {
        if (hostname && hostIp) {
          mhLink = `https://mobileharness-fe.corp.google.com/devicedetailview/${hostname}/${hostIp}/${d.id}`;
        } else {
          const urlParams = new URLSearchParams();
          urlParams.append('filter', `"id":("${d.id}")`);
          mhLink = `https://mobileharness-fe.corp.google.com/devicelistview?${urlParams.toString()}`;
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
  state: {
    orderByField: 'state',
    renderCell: ({ value, device }) => {
      const stateValue =
        device?.omnilabSpec?.labels?.[
          'dut_state'
        ]?.values?.[0]?.toUpperCase() ??
        (value as string) ??
        '';

      if (stateValue === '') return <></>;

      return renderChipCell<AndroidDevice>(
        (_1, _2) =>
          'https://g3doc.corp.google.com/company/teams/chrome/ops/fleet/flops/android/labtechs.md?cl=head#device-terminology',
        getAndroidStatusColor,
        undefined,
        true,
        stateValue.toUpperCase() as StateUnion,
      )({
        cell: { getValue: () => value },
        row: { original: device },
      } as FC_CellProps<AndroidDevice>);
    },
  },
  run_target: {
    accessorFn: (device) => device.runTarget,
    orderByField: 'run_target',
    size: 150,
    meta: {
      infoTooltip: (
        <>
          This inclues some fallbacks we use in case we dont get a{' '}
          <code>run_target</code> from omnilab. <br />
          In order:
          <ul css={{ marginTop: 0 }}>
            <li>
              <code>run_target</code>
            </li>
            <li>
              <code>product_board</code>
            </li>
            <li>
              <code>hardware</code>
            </li>
          </ul>
        </>
      ),
    },
  },
  realm: {
    accessorFn: (device) => device.realm,
    orderByField: 'realm',
  },
  fc_is_offline: {
    orderByField: 'fc_is_offline',
    meta: {
      infoTooltip: (
        <>
          Whether the device is actually offline as defined by the{' '}
          <span css={{ whiteSpace: 'nowrap' }}>
            <Link
              to="https://g3doc.corp.google.com/company/teams/omnilab/products/lmp/monitoring/alert_config_manual.md?cl=head#Q3"
              target="_blank"
            >
              omnilab docs
              <OpenInNewIcon css={{ width: '15px', height: 'auto' }} />
            </Link>
          </span>
        </>
      ),
    },
    renderCell: ({ value }) => {
      const isOffline = value === 'true';
      const title = isOffline
        ? 'Device is offline (true)'
        : 'Device is online (false)';
      return (
        <div
          css={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            maxWidth: '100%',
            height: '100%',
          }}
        >
          <Tooltip title={title}>
            {isOffline ? <PowerOffOutlinedIcon /> : <PowerIcon />}
          </Tooltip>
        </div>
      );
    },
  },
  'ufs.last_sync': {
    orderByField: 'labels.ufs.last_sync',
    renderCell: ({ value }) => {
      if (!value) {
        return null;
      }
      try {
        const dt = DateTime.fromISO(value as string);
        if (!dt.isValid) {
          return <>{value as string}</>;
        }
        return <SmartRelativeTimestamp date={dt} />;
      } catch (_) {
        return <>{value as string}</>;
      }
    },
  },
  'mh.last_sync': {
    orderByField: 'labels.mh.last_sync',
    renderCell: ({ value }) => {
      if (!value) {
        return null;
      }
      try {
        const dt = DateTime.fromISO(value as string);
        if (!dt.isValid) {
          return <>{value as string}</>;
        }
        return <SmartRelativeTimestamp date={dt} />;
      } catch (_) {
        return <>{value as string}</>;
      }
    },
  },
  fc_offline_since: {
    header: 'Offline since',
    meta: {
      infoTooltip: 'Last seen online (±10 min), per the fc_is_offline',
    },
    orderByField: 'fc_offline_since',
    accessorFn: (device) => device.fcOfflineSince,
    renderCell: ({ value }) => {
      if (!value) {
        return null;
      }
      try {
        const dt = DateTime.fromISO(value as string);
        if (!dt.isValid) {
          return <>{value as string}</>;
        }
        return <SmartRelativeTimestamp date={dt} />;
      } catch (_) {
        return <>{value as string}</>;
      }
    },
  },
};
