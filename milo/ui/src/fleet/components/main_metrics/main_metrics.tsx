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

import styled from '@emotion/styled';
import ErrorIcon from '@mui/icons-material/Error';
import WarningIcon from '@mui/icons-material/Warning';
import { Alert, Skeleton, Typography } from '@mui/material';
import { UseQueryResult } from '@tanstack/react-query';
import { ReactElement } from 'react';

import { colors } from '@/fleet/theme/colors';
import { getErrorMessage } from '@/fleet/utils/errors';
import { CountDevicesResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

const Container = styled.div`
  padding: 16px 21px;
  gap: 28;
  border: 1px solid ${colors.grey[300]};
  border-radius: 4;
`;

const HAS_RIGHT_SIBLING_STYLES = {
  borderRight: `1px solid ${colors.grey[300]}`,
  marginRight: 32,
};

interface MainMetricsProps {
  countQuery: UseQueryResult<CountDevicesResponse, unknown>;
}

// TODO: b/393624377 - Refactor this component to make it easier to test.
export function MainMetrics({ countQuery }: MainMetricsProps) {
  const getContent = () => {
    if (countQuery.isError) {
      return (
        <Alert severity="error">
          {getErrorMessage(countQuery.error, 'get the main metrics')}
        </Alert>
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
            ...HAS_RIGHT_SIBLING_STYLES,
            flexGrow: 0.2,
          }}
        >
          <Typography variant="subhead1">Total</Typography>
          <div
            css={{
              display: 'flex',
              justifyContent: 'space-around',
              marginTop: 5,
            }}
          >
            <SingleMetric
              name="Devices"
              value={countQuery.data?.total}
              loading={countQuery.isLoading}
            />
          </div>
        </div>
        <div
          css={{
            ...HAS_RIGHT_SIBLING_STYLES,
            flexGrow: 0.4,
          }}
        >
          {/* Changed Task status to Lease state as it is what we show right now,
          still confirming whether this matches with the Task state. Will be changed
          accordingly we clear out everything about this */}
          <Typography variant="subhead1">Lease state</Typography>
          <div
            css={{
              display: 'flex',
              justifyContent: 'space-around',
              marginTop: 5,
            }}
          >
            <SingleMetric
              name="Leased"
              value={countQuery.data?.taskState?.busy}
              total={countQuery.data?.total}
              loading={countQuery.isLoading}
            />
            <SingleMetric
              name="Available"
              value={countQuery.data?.taskState?.idle}
              total={countQuery.data?.total}
              loading={countQuery.isLoading}
            />
          </div>
        </div>
        <div css={{ flexGrow: 1 }}>
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
              total={countQuery.data?.total}
              loading={countQuery.isLoading}
            />
            <SingleMetric
              name="Need repair"
              value={countQuery.data?.deviceState?.needRepair}
              total={countQuery.data?.total}
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
              total={countQuery.data?.total}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              loading={countQuery.isLoading}
            />
            <SingleMetric
              name="Need manual repair"
              value={countQuery.data?.deviceState?.needManualRepair}
              total={countQuery.data?.total}
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
  total?: number;
  Icon?: ReactElement;
  loading?: boolean;
};

export function SingleMetric({
  name,
  value,
  total,
  Icon,
  loading,
}: SingleMetricProps) {
  const percentage = total && value ? value / total : -1;

  const renderPercent = () => {
    if (percentage < 0) return <></>;
    return loading ? (
      <Skeleton width={16} height={18} />
    ) : (
      <Typography variant="caption" color={colors.grey[700]}>
        {percentage.toLocaleString(undefined, {
          style: 'percent',
          maximumFractionDigits: 1,
          minimumFractionDigits: 0,
        })}
      </Typography>
    );
  };

  return (
    <div css={{ marginRight: 'auto' }}>
      <Typography variant="body2">{name}</Typography>
      <div css={{ display: 'flex', gap: 4, alignItems: 'center' }}>
        {Icon && Icon}
        {loading ? (
          <Skeleton variant="text" width={34} height={36} />
        ) : (
          <>
            <Typography variant="h3">{value || 0}</Typography>
            {total ? (
              <Typography variant="caption">{` / ${total}`}</Typography>
            ) : (
              <></>
            )}
          </>
        )}
      </div>
      {renderPercent()}
    </div>
  );
}
