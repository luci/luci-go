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
import { Divider, Typography } from '@mui/material';
import { MRT_ColumnDef } from 'material-react-table';
import { Link } from 'react-router';

import {
  ANDROID_PLATFORM,
  generateDeviceListURL,
} from '@/fleet/constants/paths';
import { ORDER_BY_PARAM_KEY, OrderByDirection } from '@/fleet/hooks/order_by';
import { colors } from '@/fleet/theme/colors';
import { getFilterQueryString } from '@/fleet/utils/search_param';
import {
  RepairMetric_Priority,
  repairMetric_PriorityToJSON,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { getPriorityIcon, type Row } from './repairs_columns.utils';

export const COLUMNS = {
  priority: {
    accessorKey: 'priority',
    header: 'Priority',
    size: 80,
    meta: {
      infoTooltip: (
        <div
          css={{
            display: 'grid',
            gridTemplateColumns: 'auto auto',
            rowGap: '4px',
          }}
        >
          <div>
            <div
              css={{
                gap: '4px',
                paddingRight: 8, // columnGap doesn't work because of the line divider
                display: 'flex',
                alignItems: 'center',
              }}
            >
              {getPriorityIcon(RepairMetric_Priority.BREACHED)}
              <Typography variant="body2">BREACHED</Typography>
            </div>
          </div>
          <Typography variant="body2">
            SLO-2 is considered broken when the offline ratio is above 10% and
            the offline ratio + peak utilization ratio is above 80%
          </Typography>
          <Divider
            css={{
              backgroundColor: 'transparent',
              gridColumn: '1 / span 99',
            }}
          />
          <div>
            <div
              css={{
                gap: '4px',
                display: 'flex',
                alignItems: 'center',
              }}
            >
              {getPriorityIcon(RepairMetric_Priority.WATCH)}
              <Typography variant="body2">WATCH</Typography>
            </div>
          </div>
          <Typography variant="body2">
            SLO-2 is considered at risk when there are offline devices
          </Typography>
          <Divider
            css={{
              backgroundColor: 'transparent',
              gridColumn: '1 / span 99',
            }}
          />
          <div>
            <div
              css={{
                gap: '4px',
                display: 'flex',
                alignContent: 'center',
              }}
            >
              {getPriorityIcon(RepairMetric_Priority.NICE)}
              {/*
              the `tick` icon is very visually bottom heavy so to look aligned
              we need to move the text a bit further down
            */}
              <Typography variant="body2" css={{ paddingTop: 3 }}>
                NICE
              </Typography>
            </div>
          </div>
          <Typography variant="body2" css={{ paddingTop: 4 }}>
            Everything else is considered nice
          </Typography>
        </div>
      ),
    },
    Cell: (x) => {
      return (
        <div
          css={{
            gap: '4px',
            display: 'flex',
            alignItems: 'center',
            height: '100%',
          }}
        >
          {getPriorityIcon(x.cell.getValue())}
          <Typography variant="body2" noWrap={true}>
            {repairMetric_PriorityToJSON(x.cell.getValue())}
          </Typography>
        </div>
      );
    },
  },
  lab_name: {
    accessorKey: 'lab_name',
    header: 'Lab Name',
    size: 60,
  },
  host_group: {
    accessorKey: 'host_group',
    header: 'Host Group',
    size: 120,
  },
  run_target: {
    accessorKey: 'run_target',
    header: 'Run Target',
    size: 60,

    meta: {
      infoTooltip: (
        <>
          Includes fallbacks in order:
          <ul css={{ marginTop: 0, paddingLeft: '1.5em' }}>
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
  minimum_repairs: {
    accessorKey: 'minimum_repairs',
    header: 'Minimum Repairs',
    sortDescFirst: true,
    size: 80,
    meta: {
      infoTooltip: 'The minimum number of repairs needed to meet SLOs',
    },
  },
  devices_offline_percentage: {
    accessorKey: 'devices_offline_percentage',
    header: 'Devices Offline %',
    sortDescFirst: true,
    size: 80,
  },
  devices_offline_ratio: {
    accessorKey: 'devices_offline_ratio',
    header: 'Offline / Total Devices',
    sortDescFirst: true,
    size: 45,
  },
  peak_usage: {
    accessorKey: 'peak_usage',
    header: 'Peak Usage',
    sortDescFirst: true,
    size: 80,
    Cell: (x) => (
      <div
        css={{
          gap: '4px',
          display: 'flex',
          alignItems: 'center',
          height: '100%',
        }}
      >
        <Typography variant="body2" noWrap={true}>
          {x.cell.getValue()}
        </Typography>
        {x.cell.getValue() >= 0 && x.row.original.total_devices && (
          <Typography variant="caption" sx={{ color: colors.grey[500] }}>
            (
            {
              // Capping the value if the percentage is higher than 100%, see b/473028358.
              (x.cell.getValue() / x.row.original.total_devices <= 1
                ? x.cell.getValue() / x.row.original.total_devices
                : 1
              ).toLocaleString('en-US', {
                style: 'percent',
              })
            }
            )
          </Typography>
        )}
      </div>
    ),

    meta: {
      infoTooltip: (
        <>
          <Typography variant="body2">
            The maximum number of busy devices in the past 14 days
          </Typography>

          <Typography
            variant="caption"
            css={{ marginTop: 10, display: 'block' }}
          >
            The percentage is calculated as the ratio of 14 day peak active
            devices to the current total number of devices and is capped at
            100%.
          </Typography>
        </>
      ),
    },
  },
  'static-omnilab_link': {
    accessorKey: 'static-omnilab_link',
    header: 'Explore in Arsenal',
    enableSorting: false,
    size: 15,
    muiTableBodyCellProps: {
      align: 'center',
    },
    Cell: (x) => {
      x.cell.getValue();
      // Double encodeURIComponent because omnilab is weird i guess
      const params = new URLSearchParams();
      if (x.row.original.lab_name)
        params.append(
          'host',
          'lab_location:include:' + encodeURIComponent(x.row.original.lab_name),
        );
      if (x.row.original.host_group)
        params.append(
          'host',
          'host_group:include:' + encodeURIComponent(x.row.original.host_group),
        );
      if (x.row.original.run_target)
        if (
          x.row
            .getValue<Row['host_group']>('host_group')
            ?.includes('crystalball')
        )
          // this team uses the run target label a bit differently so we are making an exeption for them
          params.append(
            'device',
            'product_board:include:' +
              encodeURIComponent(x.row.original.run_target),
          );
        else
          params.append(
            'device',
            'hardware:include:' + encodeURIComponent(x.row.original.run_target),
          );

      const to = `https://omnilab.corp.google.com/recovery?${params.toString()}`;

      return (
        <Link
          css={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            height: '100%',
          }}
          to={to}
          target="_blank"
        >
          <OpenInNewIcon />
        </Link>
      );
    },
  },
  'static-explore_devices': {
    accessorKey: 'static-explore_devices',
    header: 'Explore devices',
    enableSorting: false,
    size: 15,
    muiTableBodyCellProps: {
      align: 'center',
    },
    Cell: (x) => {
      const filters = {
        lab_name: [x.row.original.lab_name],
        host_group: [x.row.original.host_group],
        run_target: [x.row.original.run_target],
      };

      const params = new URLSearchParams();
      params.set(ORDER_BY_PARAM_KEY, `state ${OrderByDirection.DESC}`);

      const to = `${generateDeviceListURL(ANDROID_PLATFORM)}${getFilterQueryString(filters, params)}`;

      return (
        <Link
          css={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            height: '100%',
          }}
          to={to}
          target="_blank"
        >
          <OpenInNewIcon />
        </Link>
      );
    },
  },
} satisfies Partial<{
  [Key in keyof Row | `static-${string}`]: Key extends keyof Row
    ? MRT_ColumnDef<Row, Row[Key]> & {
        accessorKey: Key;
      }
    : MRT_ColumnDef<Row, undefined>;
}>;
