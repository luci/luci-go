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
// limitations under the License.

import { type CSSObject } from '@emotion/styled';
import ErrorIcon from '@mui/icons-material/Error';
import WarningIcon from '@mui/icons-material/Warning';
import { Alert, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import { StringListFilterCategory } from '@/fleet/components/filters/string_list_filter';
import { FilterCategory } from '@/fleet/components/filters/use_filters';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { colors } from '@/fleet/theme/colors';
import { getErrorMessage } from '@/fleet/utils/errors';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

const HAS_RIGHT_SIBLING_STYLES: CSSObject = {
  borderRight: `1px solid ${colors.grey[300]}`,
  marginRight: 16,
  paddingRight: 8,
};

const METRIC_CONTAINER_STYLES: CSSObject = {
  display: 'flex',
  justifyContent: 'flex-start',
  marginTop: 4,
  flexWrap: 'wrap',
  gap: 8,
};

export interface AndroidSummaryHeaderProps {
  filters: Record<string, FilterCategory> | undefined;
  aip160: string;
}

export function AndroidSummaryHeader({
  aip160,
  filters,
}: AndroidSummaryHeaderProps) {
  const client = useFleetConsoleClient();

  const countQuery = useQuery(
    client.CountDevices.query({
      filter: aip160,
      platform: Platform.ANDROID,
    }),
  );

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
          maxWidth: 1400,
          alignItems: 'stretch', // Ensure children stretch to fill height
          minHeight: 100,
        }}
      >
        <div
          css={{
            ...HAS_RIGHT_SIBLING_STYLES,
            flexGrow: 2, // Give more space to Device state (more metrics)
          }}
        >
          <Typography variant="subhead1">Device state</Typography>
          <div css={METRIC_CONTAINER_STYLES}>
            <SingleMetric
              name="Total"
              value={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
            />
            <SingleMetric
              name="Ready"
              value={countQuery.data?.androidCount?.idleDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              handleClick={() => {
                const f = filters?.['"state"'];
                if (f instanceof StringListFilterCategory)
                  f.setOptions({
                    ['"IDLE"']: true,
                  });
              }}
            />
            <SingleMetric
              name="Busy"
              value={countQuery.data?.androidCount?.busyDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              handleClick={() => {
                const f = filters?.['"state"'];
                if (f instanceof StringListFilterCategory)
                  f.setOptions({
                    ['"BUSY"']: true,
                  });
              }}
            />
            <SingleMetric
              name="Prepping"
              value={countQuery.data?.androidCount?.preppingDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              handleClick={() => {
                const f = filters?.['"state"'];
                if (f instanceof StringListFilterCategory)
                  f.setOptions({
                    ['"MISSING"']: true,
                  });
              }}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
            />
            <SingleMetric
              name="Dying"
              value={countQuery.data?.androidCount?.dyingDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              handleClick={() => {
                const f = filters?.['"state"'];
                if (f instanceof StringListFilterCategory)
                  f.setOptions({
                    ['"DYING"']: true,
                  });
              }}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
            />
            <SingleMetric
              name="Init"
              value={countQuery.data?.androidCount?.initDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              handleClick={() => {
                const f = filters?.['"state"'];

                if (f instanceof StringListFilterCategory)
                  f.setOptions({
                    ['"INIT"']: true,
                  });
              }}
              Icon={
                <WarningIcon
                  sx={{ color: colors.yellow[900], marginTop: '-2px' }}
                />
              }
            />
            <SingleMetric
              name="Lameduck"
              value={countQuery.data?.androidCount?.lameduckDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              handleClick={() => {
                const f = filters?.['"state"'];
                if (f instanceof StringListFilterCategory)
                  f.setOptions({
                    ['"LAMEDUCK"']: true,
                  });
              }}
            />
            <SingleMetric
              name="Failed"
              value={countQuery.data?.androidCount?.failedDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              handleClick={() => {
                const f = filters?.['"state"'];
                if (f instanceof StringListFilterCategory)
                  f.setOptions({
                    ['"FAILED"']: true,
                  });
              }}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
            />
            <SingleMetric
              name="Dirty"
              value={countQuery.data?.androidCount?.dirtyDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              handleClick={() => {
                const f = filters?.['"state"'];
                if (f instanceof StringListFilterCategory)
                  f.setOptions({
                    ['"DIRTY"']: true,
                  });
              }}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
            />
          </div>
        </div>

        <div css={{ flexGrow: 1, paddingLeft: 16 }}>
          <Typography variant="subhead1">Host state</Typography>
          <div css={METRIC_CONTAINER_STYLES}>
            <SingleMetric
              name="Total Hosts"
              value={countQuery.data?.androidCount?.totalHosts}
              loading={countQuery.isPending}
            />
            <SingleMetric
              name="Hosts Running"
              value={countQuery.data?.androidCount?.labRunningHosts}
              total={countQuery.data?.androidCount?.totalHosts}
              loading={countQuery.isPending}
              handleClick={() => {
                const state = filters?.['"state"'];
                if (state instanceof StringListFilterCategory) {
                  state.setOptions({
                    ['"LAB_RUNNING']: true,
                  });
                }

                const machineType = filters?.['"fc_machine_type"'];
                if (machineType instanceof StringListFilterCategory) {
                  machineType.setOptions({
                    ['"host']: true,
                  });
                }
              }}
            />
            <SingleMetric
              name="Hosts Missing"
              value={countQuery.data?.androidCount?.labMissingHosts}
              total={countQuery.data?.androidCount?.totalHosts}
              loading={countQuery.isPending}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              handleClick={() => {
                const state = filters?.['"state"'];
                if (state instanceof StringListFilterCategory) {
                  state.setOptions({
                    ['"LAB_MISSING"']: true,
                  });
                }

                const machineType = filters?.['"fc_machine_type"'];
                if (machineType instanceof StringListFilterCategory) {
                  machineType.setOptions({
                    ['"host']: true,
                  });
                }
              }}
            />
          </div>
        </div>
      </div>
    );
  };

  return (
    <MetricsContainer>
      <Typography variant="h4">Main metrics</Typography>
      <div css={{ marginTop: 24 }}>{getContent()}</div>
    </MetricsContainer>
  );
}
