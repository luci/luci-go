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

import { GrpcError } from '@chopsui/prpc-client';
import styled from '@emotion/styled';
import ErrorIcon from '@mui/icons-material/Error';
import WarningIcon from '@mui/icons-material/Warning';
import { Alert, Skeleton, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { ReactElement } from 'react';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { colors } from '@/fleet/theme/colors';
import { SelectedOptions } from '@/fleet/types';
import { CountDevicesRequest } from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { stringifyFilters } from '../multi_select_filter/search_param_utils/search_param_utils';

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

function getErrorMessage(error: unknown): string {
  if (error instanceof GrpcError) {
    if (error.code === 7) {
      return 'You dont have permission to get the main metrics';
    }

    return error.description;
  }
  return 'Unknown error';
}

const Container = styled.div`
  padding: 16px 21px;
  gap: 28;
  border: 1px solid ${colors.grey[300]};
  border-radius: 4;
`;

export function MainMetrics({ filter }: { filter: SelectedOptions }) {
  const client = useFleetConsoleClient();
  const countQuery = useQuery(
    client.CountDevices.query(
      CountDevicesRequest.fromPartial({
        filter: stringifyFilters(filter),
      }),
    ),
  );

  const getContent = () => {
    if (countQuery.isError) {
      return (
        <Alert severity="error">{getErrorMessage(countQuery.error)}</Alert>
      );
    }

    return (
      <div
        css={{
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
              value={countQuery.data?.taskState?.busy}
              percentage={
                countQuery.data?.taskState?.busy &&
                countQuery.data.taskState.busy / fakeData.total
              }
              loading={countQuery.isLoading}
            />
            <SingleMetric
              name="Idle"
              value={countQuery.data?.taskState?.idle}
              percentage={
                countQuery.data?.taskState?.idle &&
                countQuery.data.taskState.idle / fakeData.total
              }
              loading={countQuery.isLoading}
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
              value={countQuery.data?.deviceState?.ready}
              percentage={
                countQuery.data?.deviceState?.ready &&
                countQuery.data.deviceState.ready / fakeData.total
              }
              loading={countQuery.isLoading}
            />
            <SingleMetric
              name="Need repair"
              value={countQuery.data?.deviceState?.needRepair}
              percentage={
                countQuery.data?.deviceState?.needRepair &&
                countQuery.data.deviceState.needRepair / fakeData.total
              }
              Icon={
                <WarningIcon
                  sx={{ color: colors.yellow[900], marginTop: '-2px' }}
                />
              }
              loading={countQuery.isLoading}
            />
            <SingleMetric
              name="Repair failed"
              value={countQuery.data?.deviceState?.repairFailed}
              percentage={
                countQuery.data?.deviceState?.repairFailed &&
                countQuery.data.deviceState.repairFailed / fakeData.total
              }
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              loading={countQuery.isLoading}
            />
            <SingleMetric
              name="Need manual repair"
              value={countQuery.data?.deviceState?.needManualRepair}
              percentage={
                countQuery.data?.deviceState?.needManualRepair &&
                countQuery.data.deviceState.needManualRepair / fakeData.total
              }
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              loading={countQuery.isLoading}
            />
          </div>
        </div>
      </div>
    );
  };

  return (
    <Container>
      <Typography variant="h4">Main metrics</Typography>
      <div css={{ marginTop: 24 }}>{getContent()}</div>
    </Container>
  );
}

type SingleMetricProps = {
  name: string;
  value?: number;
  percentage?: number;
  Icon?: ReactElement;
  loading?: boolean;
};

function SingleMetric({
  name,
  value,
  percentage,
  Icon,
  loading,
}: SingleMetricProps) {
  return (
    <div css={{ marginRight: 'auto' }}>
      <Typography variant="body2">{name}</Typography>
      <div css={{ display: 'flex', gap: 4, alignItems: 'center' }}>
        {Icon && Icon}
        {!value || loading ? (
          <Skeleton variant="text" width={34} height={36} />
        ) : (
          <Typography variant="h3">{value}</Typography>
        )}
      </div>
      {!percentage || loading ? (
        <Skeleton width={16} height={18} />
      ) : (
        <Typography variant="caption" color={colors.grey[700]}>
          {percentage.toLocaleString(undefined, { style: 'percent' })}
        </Typography>
      )}
    </div>
  );
}
