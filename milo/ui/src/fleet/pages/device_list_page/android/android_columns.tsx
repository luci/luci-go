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
import PowerIcon from '@mui/icons-material/Power';
import PowerOffOutlinedIcon from '@mui/icons-material/PowerOffOutlined';
import { Tooltip, Typography } from '@mui/material';
import { DateTime } from 'luxon';
import { Link } from 'react-router';

import { DeviceTableGridColDef } from '@/fleet/components/device_table/device_table';
import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { InfoTooltip } from '@/fleet/components/info_tooltip/info_tooltip';
import { SmartRelativeTimestamp } from '@/fleet/components/smart_relative_timestamp';
import { renderChipCell } from '@/fleet/components/table/cell_with_chip';
import {
  generateDeviceDetailsURL,
  ANDROID_PLATFORM,
} from '@/fleet/constants/paths';
import { AndroidDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { getAndroidStatusColor } from './android_state';
import { RunTargetColumnHeader } from './run_target_column_header';

export const getAndroidColumns = (
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
      const stateValue =
        props.row?.omnilabSpec?.labels?.[
          'dut_state'
        ]?.values?.[0]?.toUpperCase() ??
        props.value ??
        '';

      if (stateValue === '') return <></>;

      return renderChipCell((_1, _2) => {
        return 'https://g3doc.corp.google.com/company/teams/chrome/ops/fleet/flops/android/labtechs.md?cl=head#device-terminology';
      }, getAndroidStatusColor)({ ...props, value: stateValue.toUpperCase() });
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
  realm: {
    valueGetter: (_, device) => device.realm,
    orderByField: 'realm',
  },
  lab_name: {
    orderByField: 'lab_name',
  },
  fc_machine_type: {
    orderByField: 'fc_machine_type',
  },
  fc_is_offline: {
    orderByField: 'fc_is_offline',
    renderHeader: () => {
      return (
        <div
          css={{
            display: 'flex',
            alignItems: 'center',
            maxWidth: '100%',
          }}
        >
          <Typography
            variant="subhead2"
            sx={{
              overflowX: 'hidden',
              textOverflow: 'ellipsis',
              fontWeight: 500,
            }}
          >
            fc_is_offline
          </Typography>
          <InfoTooltip infoCss={{ marginLeft: '10px' }}>
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
          </InfoTooltip>
        </div>
      );
    },
    renderCell: (params) => {
      const isOffline = params.value === 'true';
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
  fc_offline_since: {
    headerName: 'Offline since',
    renderHeader: () => {
      return (
        <div
          css={{
            display: 'flex',
            alignItems: 'center',
            maxWidth: '100%',
          }}
        >
          <Typography
            variant="subhead2"
            sx={{
              overflowX: 'hidden',
              textOverflow: 'ellipsis',
              fontWeight: 500,
            }}
          >
            Offline since
          </Typography>
          <InfoTooltip infoCss={{ marginLeft: '10px' }}>
            Last seen online (±10 min), per the fc_is_offline
            <br />
          </InfoTooltip>
        </div>
      );
    },
    orderByField: 'fc_offline_since',
    renderCell: (params) => {
      const value = params.value as string | undefined;
      if (!value) {
        return null;
      }
      const dt = DateTime.fromISO(value);
      if (!dt.isValid) {
        return <>{value}</>;
      }
      return <SmartRelativeTimestamp date={dt} />;
    },
    valueGetter: (_, device) => device.fcOfflineSince,
  },
};
