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
import { Link as MuiLink, Tooltip, Typography, Box } from '@mui/material';
import { DateTime } from 'luxon';
import { MRT_ColumnDef } from 'material-react-table';
import React from 'react';
import { Link } from 'react-router';

import { genFeedbackUrl } from '@/common/tools/utils';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { SmartRelativeTimestamp } from '@/fleet/components/smart_relative_timestamp';
import { BuganizerLink } from '@/fleet/components/table/buganizer_link';
import {
  renderChipCell,
  StateUnion,
} from '@/fleet/components/table/cell_with_chip';
import { getEnabledFeatureFlags } from '@/fleet/config/features';
import { FEEDBACK_BUGANIZER_BUG_ID } from '@/fleet/constants/feedback';
import {
  generateDeviceDetailsURL,
  ANDROID_PLATFORM,
  PIXEL_PLATFORM,
} from '@/fleet/constants/paths';
import { FC_CellProps } from '@/fleet/types/table';
import { AndroidPageWorkspace } from '@/fleet/workspaces';
import { AndroidDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getAndroidStatusColor } from './android_state';

export type AndroidColumnDef = MRT_ColumnDef<AndroidDevice> & {
  orderByField?: string;
  filterKey?: string;
};

export interface DeviceDisplayProps {
  value: unknown;
  device?: AndroidDevice;
  workspace: AndroidPageWorkspace;
}

export type AndroidColumnOverride = Omit<Partial<AndroidColumnDef>, 'Cell'> & {
  renderCell?: (props: DeviceDisplayProps) => React.ReactNode;
};

const renderTimestamp = ({ value }: DeviceDisplayProps) => {
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
};

export const genUtilizationFeedbackUrl = () =>
  genFeedbackUrl({
    bugComponent: FEEDBACK_BUGANIZER_BUG_ID,
    customComment:
      `# Device Utilization Problem Description\n\n` +
      `Please describe the discrepancy or issue you observed with the device utilization metrics.\n\n` +
      `# Discrepancy Details\n\n` +
      `- Expected utilization / behavior:\n` +
      `- Actual utilization shown:\n\n` +
      `# Screenshot & Network Calls\n\n` +
      `Please attach a screenshot of the incorrect values and network tab if possible, as this will be super helpful for debugging.\n\n` +
      `# Autopopulated Info\n\n` +
      `- **Version**: ${UI_VERSION}\n` +
      `- **From Link**: ${self.location.href}\n` +
      `- **User Agent**: ${navigator.userAgent}\n` +
      `- **Enabled Feature Flags**: ${getEnabledFeatureFlags().join(', ') || 'None'}\n`,
  });

export const UtilizationTooltipContent = ({
  isSummary,
}: {
  isSummary?: boolean;
}) => (
  <Box
    sx={{ display: 'flex', flexDirection: 'column', gap: 1, maxWidth: '350px' }}
  >
    <Typography variant="body2" fontWeight="bold">
      Device utilization (Beta)
    </Typography>
    <Typography variant="body2">
      {isSummary
        ? 'Shows the average utilization for devices matching your search. Excludes devices with no data.'
        : 'Shows the average utilization for the device over the specified time period.'}
    </Typography>

    <Typography variant="body2" fontWeight="bold">
      Calculation
    </Typography>
    <Typography variant="body2">
      The percentage of total time a device actively runs tests (BUSY).
    </Typography>

    <Typography variant="body2" fontWeight="bold">
      Device health limits
    </Typography>
    <Typography variant="body2">
      This calculation currently includes downtime. A low rate might mean the
      device is offline or broken, rather than idle. It does not yet check the
      device&apos;s OperationalState (such as OFFLINE, TRANSIENT, or
      NEED_MANUAL_REPAIR).
    </Typography>

    <Typography variant="body2" fontWeight="bold">
      Feedback
    </Typography>
    <Typography variant="body2">
      <MuiLink
        href={genUtilizationFeedbackUrl()}
        target="_blank"
        rel="noopener noreferrer"
      >
        Report data issues to help us improve. Attaching screenshots with
        incorrect values will help us with debugging!
      </MuiLink>
    </Typography>
  </Box>
);

export const ANDROID_COLUMN_OVERRIDES: Record<string, AndroidColumnOverride> = {
  id: {
    size: 180,
    minSize: 120,
    orderByField: 'id',
    accessorFn: (device) => device.id,

    renderCell: ({ value, device, workspace }) => {
      const d = device;
      if (!d) return (value as React.ReactNode) ?? null;
      const internalLink = generateDeviceDetailsURL(
        workspace === 'Android' ? ANDROID_PLATFORM : PIXEL_PLATFORM,
        d.id,
      );

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
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
            maxWidth: '100%',
          }}
        >
          <BuganizerLink name={d.id} project="android" />
          <Link
            to={internalLink}
            style={{
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              minWidth: 0,
            }}
          >
            {d.id}
          </Link>
          <Tooltip title="Open in Mobile Harness">
            <a
              href={mhLink}
              target="_blank"
              rel="noopener noreferrer"
              style={{ display: 'flex', alignItems: 'center' }}
            >
              <OpenInNewIcon fontSize="small" />
            </a>
          </Tooltip>
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

      return renderChipCell<AndroidDevice>({
        getValueOrUrl: (_1, _2) =>
          'https://g3doc.corp.google.com/company/teams/chrome/ops/fleet/flops/android/labtechs.md?cl=head#device-terminology',
        getColor: getAndroidStatusColor,
        overrideValue: stateValue.toUpperCase() as StateUnion,
        getTrackingEvent: (value) => ({
          eventName: 'state_doc_link_clicked',
          payload: { componentName: 'android_dut_state', activeTab: value },
        }),
      })({
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
          <ul style={{ marginTop: 0 }}>
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
          <span style={{ whiteSpace: 'nowrap' }}>
            <Link
              to="https://g3doc.corp.google.com/company/teams/omnilab/products/lmp/monitoring/alert_config_manual.md?cl=head#Q3"
              target="_blank"
            >
              omnilab docs
              <OpenInNewIcon style={{ width: '15px', height: 'auto' }} />
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
          style={{
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
    renderCell: renderTimestamp,
  },
  'mh.last_sync': {
    orderByField: 'labels.mh.last_sync',
    renderCell: renderTimestamp,
  },
  'ufs.nlyte_update_time': {
    header: 'Nlyte Update Time',
    orderByField: 'labels.ufs.nlyte_update_time',
    meta: {
      infoTooltip: 'Time when the Nlyte data last changed',
    },
    renderCell: renderTimestamp,
  },
  'ufs.nlyte_last_sync': {
    header: 'Nlyte Last Sync',
    orderByField: 'labels.ufs.nlyte_last_sync',
    meta: {
      infoTooltip: 'Time that UFS ran the cron and checked for updates',
    },
    renderCell: renderTimestamp,
  },
  fc_offline_since: {
    //TODO (b/502485099): this will be filterable after is resolved
    header: 'Offline since',
    meta: {
      infoTooltip: 'Last seen online (±10 min), per the fc_is_offline',
    },
    orderByField: 'fc_offline_since',
    accessorFn: (device) => device.fcOfflineSince,
    renderCell: renderTimestamp,
  },
  average_7d: {
    //TODO (b/502485099): this will be filterable after is resolved
    header: '7 Day Average Utilization',
    meta: {
      infoTooltip: <UtilizationTooltipContent />,
    },
    orderByField: 'average_7d',
    accessorFn: (device) =>
      device.average7d !== undefined && device.average7d !== null
        ? device.average7d * 100
        : undefined,
    filterVariant: 'range-slider',
    muiFilterSliderProps: {
      min: 0,
      max: 100,
    },
    renderCell: ({ value, device }) => {
      const val =
        value ??
        (device?.average7d !== undefined && device?.average7d !== null
          ? device.average7d * 100
          : null);
      if (typeof val !== 'number') return null;
      return <>{val.toFixed(2)}%</>;
    },
  },
  average_30d: {
    //TODO (b/502485099): this will be filterable after is resolved
    header: '30 Day Average Utilization',
    meta: {
      infoTooltip: <UtilizationTooltipContent />,
    },
    orderByField: 'average_30d',
    accessorFn: (device) =>
      device.average30d !== undefined && device.average30d !== null
        ? device.average30d * 100
        : undefined,
    filterVariant: 'range-slider',
    muiFilterSliderProps: {
      min: 0,
      max: 100,
    },
    renderCell: ({ value, device }) => {
      const val =
        value ??
        (device?.average30d !== undefined && device?.average30d !== null
          ? device.average30d * 100
          : null);
      if (typeof val !== 'number') return null;
      return <>{val.toFixed(2)}%</>;
    },
  },
  battery_temperature: {
    header: 'Battery Temperature',
    orderByField: 'labels.battery_temperature',
    renderCell: ({ value }) => {
      if (value === undefined || value === null || value === '') {
        return null;
      }
      const strVal = String(value);
      const formatted = strVal.endsWith('°C') ? strVal : `${strVal} °C`;
      return <EllipsisTooltip>{formatted}</EllipsisTooltip>;
    },
  },
};
