// Copyright 2024 The LUCI Authors.
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

import ErrorIcon from '@mui/icons-material/Error';
import WarningIcon from '@mui/icons-material/Warning';
import { Typography } from '@mui/material';
import { ReactElement } from 'react';

import { colors } from '@/fleet/theme/colors';

type CountDevicesResponse = {
  total: number;

  taskState: {
    busy: number;
    idle: number;
  };
  deviceState: {
    ready: number;
    needManualRepair: number;
    needRepair: number;
    repairFailed: number;
  };
};

const fakeData: CountDevicesResponse = {
  total: 440,

  taskState: {
    busy: 401,
    idle: 40,
  },
  deviceState: {
    ready: 40,
    needManualRepair: 40,
    needRepair: 40,
    repairFailed: 40,
  },
};

export function MainMetrics() {
  return (
    <div
      css={{
        padding: '16px 21px',
        margin: '24px 26px',
        gap: 28,
        border: `solid ${colors.grey[300]}`,
        borderRadius: 4,
      }}
    >
      <Typography variant="h4">Main metrics</Typography>
      <div
        css={{
          marginTop: 24,
          display: 'flex',
          maxWidth: 1100,
        }}
      >
        <div
          css={{
            borderRight: `1px solid ${colors.grey[300]}`,
            flexGrow: 0.4,
          }}
        >
          <Typography variant="subhead1">Task status</Typography>
          <div
            css={{
              display: 'flex',
              justifyContent: 'space-around',
              marginTop: 5,
            }}
          >
            <SingleMetric
              name="Busy"
              value={fakeData.taskState.busy}
              percentage={fakeData.taskState.busy / fakeData.total}
            />
            <SingleMetric
              name="Idle"
              value={fakeData.taskState.idle}
              percentage={fakeData.taskState.idle / fakeData.total}
            />
          </div>
        </div>
        <div css={{ paddingLeft: 32, flexGrow: 1 }}>
          <Typography variant="subhead1">Device state</Typography>
          <div
            css={{
              display: 'flex',
              marginTop: 5,
              marginLeft: 8,
            }}
          >
            <SingleMetric
              name="Ready"
              value={fakeData.deviceState.ready}
              percentage={fakeData.deviceState.ready / fakeData.total}
            />
            <SingleMetric
              name="Need repair"
              value={fakeData.deviceState.needRepair}
              percentage={fakeData.deviceState.needRepair / fakeData.total}
              Icon={
                <WarningIcon
                  sx={{ color: colors.yellow[900], marginTop: '-2px' }}
                />
              }
            />
            <SingleMetric
              name="Repair failed"
              value={fakeData.deviceState.repairFailed}
              percentage={fakeData.deviceState.repairFailed / fakeData.total}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
            />
            <SingleMetric
              name="Need manual repair"
              value={fakeData.deviceState.needManualRepair}
              percentage={
                fakeData.deviceState.needManualRepair / fakeData.total
              }
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
            />
          </div>
        </div>
      </div>
    </div>
  );
}

function SingleMetric({
  name,
  value,
  percentage,
  Icon,
}: {
  name: string;
  value: number;
  percentage: number;
  Icon?: ReactElement;
}) {
  return (
    <div css={{ marginRight: 'auto' }}>
      <Typography variant="body2">{name}</Typography>
      <div css={{ display: 'flex', gap: 4, alignItems: 'center' }}>
        {Icon && Icon}
        <Typography variant="h3">{value}</Typography>
      </div>
      <Typography variant="caption" color={colors.grey[700]}>
        {percentage.toLocaleString(undefined, { style: 'percent' })}
      </Typography>
    </div>
  );
}
